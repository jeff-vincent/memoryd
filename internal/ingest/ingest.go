package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"

	"github.com/kindling-sh/memoryd/internal/chunker"
	"github.com/kindling-sh/memoryd/internal/crawler"
	"github.com/kindling-sh/memoryd/internal/embedding"
	"github.com/kindling-sh/memoryd/internal/redact"
	"github.com/kindling-sh/memoryd/internal/store"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SourceExtensionThreshold: if a new chunk is this similar to a source memory,
// tag it as extending that source rather than creating a standalone entry.
const SourceExtensionThreshold = 0.75

// Ingester crawls a URL and stores the content as source-attributed memories.
type Ingester struct {
	embedder    embedding.Embedder
	store       store.Store
	sourceStore store.SourceStore
}

// NewIngester creates an ingester with the required dependencies.
func NewIngester(emb embedding.Embedder, st store.Store, ss store.SourceStore) *Ingester {
	return &Ingester{embedder: emb, store: st, sourceStore: ss}
}

// IngestSource crawls the given source and stores its content.
// It handles change detection via content hashes and only re-ingests changed pages.
func (ing *Ingester) IngestSource(ctx context.Context, src store.Source) error {
	sourceLabel := "source:" + src.Name

	log.Printf("[ingest] starting crawl of %s (name=%s, max_depth=%d, max_pages=%d)",
		src.BaseURL, src.Name, src.MaxDepth, src.MaxPages)

	pages, err := crawler.Crawl(ctx, src.BaseURL, crawler.Options{
		MaxDepth: src.MaxDepth,
		MaxPages: src.MaxPages,
		Headers:  src.Headers,
	})
	if err != nil {
		return fmt.Errorf("crawl failed: %w", err)
	}

	log.Printf("[ingest] crawled %d pages from %s", len(pages), src.BaseURL)

	sourceID, _ := primitive.ObjectIDFromHex(src.ID.Hex())
	var memoryCount int

	for _, page := range pages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if this page has changed since last crawl.
		existing, err := ing.sourceStore.GetSourcePage(ctx, sourceID, page.URL)
		if err != nil {
			log.Printf("[ingest] page lookup error for %s: %v", page.URL, err)
		}

		if existing != nil && existing.ContentHash == page.ContentHash {
			log.Printf("[ingest] unchanged: %s", page.URL)
			// Count existing memories for this page so the total stays accurate.
			count, _ := ing.store.CountBySource(ctx, sourceLabel+"|"+page.URL)
			memoryCount += int(count)
			continue
		}

		// Page is new or changed. Delete old memories from this page.
		if existing != nil {
			oldSource := sourceLabel + "|" + page.URL
			if err := ing.sourceStore.DeleteMemoriesBySource(ctx, oldSource); err != nil {
				log.Printf("[ingest] cleanup error for %s: %v", page.URL, err)
			}
		}

		// Chunk and embed the page content.
		chunks := chunker.Chunk(page.Content, chunker.DefaultMaxTokens)
		var validChunks []string
		for _, chunk := range chunks {
			if len(strings.TrimSpace(chunk)) < 30 {
				continue
			}
			validChunks = append(validChunks, redact.Clean(chunk))
		}

		if len(validChunks) == 0 {
			continue
		}

		vecs, err := ing.embedder.EmbedBatch(ctx, validChunks)
		if err != nil {
			log.Printf("[ingest] batch embed error for %s: %v", page.URL, err)
			continue
		}

		for i, chunk := range validChunks {
			mem := store.Memory{
				Content:   chunk,
				Embedding: vecs[i],
				Source:    sourceLabel + "|" + page.URL,
				Metadata: map[string]any{
					"source_name": src.Name,
					"page_url":    page.URL,
				},
			}
			if err := ing.store.Insert(ctx, mem); err != nil {
				log.Printf("[ingest] store error: %v", err)
				continue
			}
			memoryCount++
		}

		// Record the page for change detection.
		if err := ing.sourceStore.UpsertSourcePage(ctx, store.SourcePage{
			SourceID:    sourceID,
			URL:         page.URL,
			ContentHash: page.ContentHash,
		}); err != nil {
			log.Printf("[ingest] page record error: %v", err)
		}
	}

	// Update source status.
	if err := ing.sourceStore.UpdateSourceStatus(ctx, src.ID.Hex(), "ready", "", len(pages), memoryCount); err != nil {
		log.Printf("[ingest] status update error: %v", err)
	}

	log.Printf("[ingest] done: %d pages, %d memories stored for %s", len(pages), memoryCount, src.Name)
	return nil
}

// RemoveSource deletes all memories and pages for a source, then the source itself.
func (ing *Ingester) RemoveSource(ctx context.Context, sourceID string, sourceName string) error {
	sourceLabel := "source:" + sourceName
	if err := ing.sourceStore.DeleteMemoriesBySource(ctx, sourceLabel); err != nil {
		return fmt.Errorf("delete memories: %w", err)
	}
	oid, err := primitive.ObjectIDFromHex(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source ID: %w", err)
	}
	if err := ing.sourceStore.DeleteSourcePages(ctx, oid); err != nil {
		return fmt.Errorf("delete pages: %w", err)
	}
	return ing.sourceStore.DeleteSource(ctx, sourceID)
}

// FileContent represents a single uploaded document.
type FileContent struct {
	Filename string
	Content  string
}

// IngestFiles chunks, embeds, and stores pre-read documents as a source.
func (ing *Ingester) IngestFiles(ctx context.Context, src store.Source, files []FileContent) error {
	sourceLabel := "source:" + src.Name
	sourceID, _ := primitive.ObjectIDFromHex(src.ID.Hex())

	log.Printf("[ingest] uploading %d files for source %s", len(files), src.Name)

	var memoryCount int
	for _, f := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		content := strings.TrimSpace(f.Content)
		if len(content) < 50 {
			log.Printf("[ingest] skip %s: too short (%d chars)", f.Filename, len(content))
			continue
		}

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		fileLabel := sourceLabel + "|" + f.Filename

		// Change detection: skip if this file hasn't changed.
		existing, _ := ing.sourceStore.GetSourcePage(ctx, sourceID, f.Filename)
		if existing != nil && existing.ContentHash == hash {
			log.Printf("[ingest] unchanged: %s", f.Filename)
			count, _ := ing.store.CountBySource(ctx, fileLabel)
			memoryCount += int(count)
			continue
		}

		// File is new or changed — delete old memories for this file.
		if existing != nil {
			if err := ing.sourceStore.DeleteMemoriesBySource(ctx, fileLabel); err != nil {
				log.Printf("[ingest] delete old memories error for %s: %v", fileLabel, err)
			}
		}

		chunks := chunker.Chunk(content, chunker.DefaultMaxTokens)
		var validChunks []string
		for _, chunk := range chunks {
			if len(strings.TrimSpace(chunk)) < 30 {
				continue
			}
			validChunks = append(validChunks, redact.Clean(chunk))
		}
		if len(validChunks) == 0 {
			continue
		}

		vecs, err := ing.embedder.EmbedBatch(ctx, validChunks)
		if err != nil {
			log.Printf("[ingest] batch embed error for %s: %v", f.Filename, err)
			continue
		}

		for i, chunk := range validChunks {
			mem := store.Memory{
				Content:   chunk,
				Embedding: vecs[i],
				Source:    fileLabel,
				Metadata: map[string]any{
					"source_name": src.Name,
					"filename":    f.Filename,
				},
			}
			if err := ing.store.Insert(ctx, mem); err != nil {
				log.Printf("[ingest] store error: %v", err)
				continue
			}
			memoryCount++
		}

		// Record the file for change detection.
		if err := ing.sourceStore.UpsertSourcePage(ctx, store.SourcePage{
			SourceID:    sourceID,
			URL:         f.Filename,
			ContentHash: hash,
		}); err != nil {
			log.Printf("[ingest] upsert page error for %s: %v", f.Filename, err)
		}
	}

	if err := ing.sourceStore.UpdateSourceStatus(ctx, src.ID.Hex(), "ready", "", len(files), memoryCount); err != nil {
		log.Printf("[ingest] status update error: %v", err)
	}

	log.Printf("[ingest] done: %d files, %d memories stored for %s", len(files), memoryCount, src.Name)
	return nil
}
