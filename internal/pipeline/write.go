package pipeline

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/memory-daemon/memoryd/internal/chunker"
	"github.com/memory-daemon/memoryd/internal/embedding"
	"github.com/memory-daemon/memoryd/internal/redact"
	"github.com/memory-daemon/memoryd/internal/store"
)

// DedupThreshold is the cosine-similarity score above which a new chunk
// is considered a duplicate of an existing memory.
const DedupThreshold = 0.92

// SourceExtensionThreshold: if a new chunk scores this high against a source
// memory (but below DedupThreshold), tag it as extending that source.
const SourceExtensionThreshold = 0.75

// minContentLen is the minimum character length for content to be worth storing.
const minContentLen = 20

// WriteResult reports what the write pipeline did with the input.
type WriteResult struct {
	Stored     int // chunks actually inserted
	Duplicates int // chunks skipped as near-duplicates
	Filtered   int // chunks skipped as noise
	Extended   int // chunks that extend source content
}

func (r WriteResult) Summary() string {
	parts := []string{}
	if r.Stored > 0 {
		parts = append(parts, fmt.Sprintf("%d stored", r.Stored))
	}
	if r.Extended > 0 {
		parts = append(parts, fmt.Sprintf("%d extend source", r.Extended))
	}
	if r.Duplicates > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (duplicate)", r.Duplicates))
	}
	if r.Filtered > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (too short/noisy)", r.Filtered))
	}
	if len(parts) == 0 {
		return "Nothing to store."
	}
	return strings.Join(parts, ", ") + "."
}

// WritePipeline handles the post-response path: chunk the text,
// embed each chunk, and write to the store. Designed to run in a goroutine.
type WritePipeline struct {
	embedder embedding.Embedder
	store    store.Store
}

func NewWritePipeline(e embedding.Embedder, s store.Store) *WritePipeline {
	return &WritePipeline{embedder: e, store: s}
}

// Process chunks, embeds, deduplicates, and stores the text.
// Safe to call from a goroutine -- errors are logged, not returned.
func (wp *WritePipeline) Process(text, source string, metadata map[string]any) {
	wp.ProcessFiltered(text, source, metadata)
}

// ProcessFiltered is like Process but returns a WriteResult describing
// what happened (stored, deduplicated, filtered).
func (wp *WritePipeline) ProcessFiltered(text, source string, metadata map[string]any) WriteResult {
	var result WriteResult

	if wp.embedder == nil || wp.store == nil {
		return result
	}

	ctx := context.Background()

	chunks := chunker.Chunk(text, chunker.DefaultMaxTokens)

	// First pass: filter noise and redact, collect valid chunks.
	var validChunks []string
	for _, chunk := range chunks {
		if isNoise(chunk) {
			result.Filtered++
			log.Printf("[write] filtered noisy chunk (%d chars)", len(chunk))
			continue
		}
		validChunks = append(validChunks, redact.Clean(chunk))
	}

	if len(validChunks) == 0 {
		return result
	}

	// Batch-embed all valid chunks in a single call.
	vecs, err := wp.embedder.EmbedBatch(ctx, validChunks)
	if err != nil {
		log.Printf("[write] batch embedding error: %v", err)
		return result
	}

	// Second pass: dedup and store using pre-computed embeddings.
	for i, chunk := range validChunks {
		vec := vecs[i]

		isDup, closest := wp.checkDuplicate(ctx, vec)
		if isDup {
			result.Duplicates++
			log.Printf("[write] skipped duplicate chunk")
			continue
		}

		chunkMeta := metadata
		if closest != nil && strings.HasPrefix(closest.Source, "source:") &&
			closest.Score >= SourceExtensionThreshold {
			if chunkMeta == nil {
				chunkMeta = map[string]any{}
			} else {
				copied := make(map[string]any, len(chunkMeta)+3)
				for k, v := range chunkMeta {
					copied[k] = v
				}
				chunkMeta = copied
			}
			chunkMeta["extends_source"] = closest.Source
			chunkMeta["extends_memory"] = closest.ID.Hex()
			chunkMeta["extends_score"] = closest.Score
			result.Extended++
			log.Printf("[write] tagged as extending source %s (score %.2f)", closest.Source, closest.Score)
		}

		mem := store.Memory{
			Content:   chunk,
			Embedding: vec,
			Source:    source,
			Metadata:  chunkMeta,
		}

		if err := wp.store.Insert(ctx, mem); err != nil {
			log.Printf("[write] store error: %v", err)
			continue
		}
		result.Stored++
	}
	return result
}

// checkDuplicate returns whether the vector is a duplicate and the closest match.
func (wp *WritePipeline) checkDuplicate(ctx context.Context, vec []float32) (bool, *store.Memory) {
	matches, err := wp.store.VectorSearch(ctx, vec, 1)
	if err != nil {
		log.Printf("[write] dedup search error (storing anyway): %v", err)
		return false, nil
	}
	if len(matches) == 0 {
		return false, nil
	}
	if matches[0].Score >= DedupThreshold {
		return true, &matches[0]
	}
	return false, &matches[0]
}

// isNoise returns true if the text is too short, mostly whitespace,
// or lacks meaningful alphanumeric content.
func isNoise(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < minContentLen {
		return true
	}

	// Count alphanumeric characters.
	var alnumCount int
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alnumCount++
		}
	}

	// If less than 40% of characters are alphanumeric, it's noise.
	if float64(alnumCount)/float64(len(trimmed)) < 0.4 {
		return true
	}

	return false
}
