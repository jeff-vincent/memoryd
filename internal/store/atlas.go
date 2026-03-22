package store

import (
	"context"
	"fmt"
	"math"
	"regexp"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// AtlasStore wraps MongoStore with features only available on Atlas proper:
// pre-filtered vector search, full-text $search, hybrid retrieval, and MMR.
type AtlasStore struct {
	*MongoStore
}

// NewAtlasStore creates an Atlas-enhanced store. The underlying connection
// must point to an Atlas cluster (not community/local) for hybrid features.
func NewAtlasStore(ctx context.Context, uri, database string) (*AtlasStore, error) {
	base, err := NewMongoStore(ctx, uri, database)
	if err != nil {
		return nil, err
	}
	return &AtlasStore{MongoStore: base}, nil
}

// VectorSearch overrides MongoStore.VectorSearch with quality pre-filtering.
// Memories with quality_score > 0 and below 0.05 are excluded (garbage tier).
func (a *AtlasStore) VectorSearch(ctx context.Context, embedding []float32, topK int) ([]Memory, error) {
	return a.filteredVectorSearch(ctx, embedding, topK, 0.05, "")
}

// filteredVectorSearch runs $vectorSearch with optional pre-filter on quality_score and source.
func (a *AtlasStore) filteredVectorSearch(ctx context.Context, embedding []float32, topK int, minQuality float64, source string) ([]Memory, error) {
	vsStage := bson.D{
		{Key: "index", Value: "vector_index"},
		{Key: "path", Value: "embedding"},
		{Key: "queryVector", Value: embedding},
		{Key: "numCandidates", Value: topK * 20},
		{Key: "limit", Value: topK},
	}

	// Build pre-filter if needed.
	var filters []bson.D
	if minQuality > 0 {
		// Include memories that either have no quality_score yet (new) or score above threshold.
		filters = append(filters, bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "quality_score", Value: bson.D{{Key: "$gte", Value: minQuality}}}},
			bson.D{{Key: "quality_score", Value: bson.D{{Key: "$eq", Value: 0}}}},
		}}})
	}
	if source != "" {
		filters = append(filters, bson.D{{Key: "source", Value: bson.D{
			{Key: "$regex", Value: "^" + regexp.QuoteMeta(source)},
		}}})
	}

	if len(filters) == 1 {
		vsStage = append(vsStage, bson.E{Key: "filter", Value: filters[0]})
	} else if len(filters) > 1 {
		vsStage = append(vsStage, bson.E{Key: "filter", Value: bson.D{{Key: "$and", Value: filters}}})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$vectorSearch", Value: vsStage}},
		{{Key: "$project", Value: bson.D{
			{Key: "content", Value: 1},
			{Key: "source", Value: 1},
			{Key: "metadata", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "hit_count", Value: 1},
			{Key: "quality_score", Value: 1},
			{Key: "embedding", Value: 1},
			{Key: "score", Value: bson.D{{Key: "$meta", Value: "vectorSearchScore"}}},
		}}},
	}

	cursor, err := a.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("atlas vector search: %w", err)
	}
	defer cursor.Close(ctx)

	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decoding atlas results: %w", err)
	}
	return results, nil
}

// textSearch runs Atlas $search (Lucene full-text) and returns scored results.
func (a *AtlasStore) textSearch(ctx context.Context, query string, topK int) ([]Memory, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$search", Value: bson.D{
			{Key: "index", Value: "text_index"},
			{Key: "text", Value: bson.D{
				{Key: "query", Value: query},
				{Key: "path", Value: "content"},
			}},
		}}},
		{{Key: "$limit", Value: topK}},
		{{Key: "$project", Value: bson.D{
			{Key: "content", Value: 1},
			{Key: "source", Value: 1},
			{Key: "metadata", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "hit_count", Value: 1},
			{Key: "quality_score", Value: 1},
			{Key: "embedding", Value: 1},
			{Key: "score", Value: bson.D{{Key: "$meta", Value: "searchScore"}}},
		}}},
	}

	cursor, err := a.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("atlas text search: %w", err)
	}
	defer cursor.Close(ctx)

	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// HybridSearch combines vector similarity with optional text matching,
// quality pre-filtering, and MMR diversity re-ranking.
//
// Strategy:
//  1. Run filtered $vectorSearch (always).
//  2. If TextQuery is set, also run $search and merge via Reciprocal Rank Fusion.
//  3. If DiversityMMR is set, re-rank final candidates via MMR.
func (a *AtlasStore) HybridSearch(ctx context.Context, embedding []float32, topK int, opts SearchOptions) ([]Memory, error) {
	// Fetch more candidates than requested so MMR/fusion has room to diversify.
	fetchK := topK * 4
	if fetchK < 20 {
		fetchK = 20
	}

	// Phase 1: filtered vector search.
	vectorResults, err := a.filteredVectorSearch(ctx, embedding, fetchK, opts.MinQualityScore, opts.Source)
	if err != nil {
		return nil, err
	}

	var merged []Memory

	if opts.TextQuery != "" {
		// Phase 2: text search.
		textResults, err := a.textSearch(ctx, opts.TextQuery, fetchK)
		if err != nil {
			// Text search failure is non-fatal; fall back to vector-only.
			merged = vectorResults
		} else {
			// Reciprocal Rank Fusion: merge the two ranked lists.
			merged = reciprocalRankFusion(vectorResults, textResults, 60)
		}
	} else {
		merged = vectorResults
	}

	// Phase 3: MMR re-ranking for diversity.
	if opts.DiversityMMR && len(merged) > topK {
		lambda := opts.MMRLambda
		if lambda == 0 {
			lambda = 0.7
		}
		merged = mmrRerank(merged, embedding, topK, lambda)
	}

	// Trim to requested topK.
	if len(merged) > topK {
		merged = merged[:topK]
	}

	return merged, nil
}

// reciprocalRankFusion merges two ranked lists using RRF.
// k is the smoothing constant (typically 60).
func reciprocalRankFusion(listA, listB []Memory, k int) []Memory {
	type scored struct {
		mem   Memory
		score float64
	}

	byID := map[string]*scored{}

	for rank, m := range listA {
		id := m.ID.Hex()
		s := 1.0 / float64(rank+k+1)
		if existing, ok := byID[id]; ok {
			existing.score += s
		} else {
			byID[id] = &scored{mem: m, score: s}
		}
	}

	for rank, m := range listB {
		id := m.ID.Hex()
		s := 1.0 / float64(rank+k+1)
		if existing, ok := byID[id]; ok {
			existing.score += s
		} else {
			byID[id] = &scored{mem: m, score: s}
		}
	}

	// Sort by RRF score descending.
	results := make([]Memory, 0, len(byID))
	scores := make([]float64, 0, len(byID))
	for _, s := range byID {
		results = append(results, s.mem)
		scores = append(scores, s.score)
	}

	// Simple insertion sort (lists are small, typically < 100).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && scores[j] > scores[j-1]; j-- {
			results[j], results[j-1] = results[j-1], results[j]
			scores[j], scores[j-1] = scores[j-1], scores[j]
		}
	}

	// Assign fused score to the Score field for downstream use.
	for i := range results {
		results[i].Score = scores[i]
	}

	return results
}

// mmrRerank applies Maximal Marginal Relevance to select diverse results.
// lambda controls relevance (1.0) vs diversity (0.0).
func mmrRerank(candidates []Memory, queryVec []float32, topK int, lambda float64) []Memory {
	if len(candidates) <= topK {
		return candidates
	}

	selected := make([]Memory, 0, topK)
	remaining := make([]int, len(candidates)) // indices into candidates
	for i := range remaining {
		remaining[i] = i
	}

	// Pre-compute query similarities.
	querySims := make([]float64, len(candidates))
	for i, c := range candidates {
		if len(c.Embedding) > 0 {
			querySims[i] = cosineSim(queryVec, c.Embedding)
		} else {
			querySims[i] = c.Score // fallback to search score
		}
	}

	for len(selected) < topK && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := math.Inf(-1)

		for ri, ci := range remaining {
			relevance := querySims[ci]

			// Max similarity to any already-selected memory.
			var maxSim float64
			for _, sel := range selected {
				if len(candidates[ci].Embedding) > 0 && len(sel.Embedding) > 0 {
					sim := cosineSim(candidates[ci].Embedding, sel.Embedding)
					if sim > maxSim {
						maxSim = sim
					}
				}
			}

			mmr := lambda*relevance - (1-lambda)*maxSim
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = ri
			}
		}

		if bestIdx < 0 {
			break
		}

		ci := remaining[bestIdx]
		selected = append(selected, candidates[ci])
		// Remove from remaining (swap with last).
		remaining[bestIdx] = remaining[len(remaining)-1]
		remaining = remaining[:len(remaining)-1]
	}

	return selected
}

// cosineSim computes cosine similarity between two vectors.
func cosineSim(a []float32, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
