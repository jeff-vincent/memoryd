package quality

import (
	"context"
	"math"
	"time"

	"github.com/memory-daemon/memoryd/internal/embedding"
)

// qualityProtos are generic descriptions of high-value knowledge chunks.
// They are embedded once at startup and used to score incoming chunks by
// cosine similarity — no domain-specific rules needed.
var qualityProtos = []string{
	"important technical decision with reasoning and rationale",
	"architecture pattern approach and implementation details",
	"debugging solution root cause analysis and fix",
	"configuration setup deployment and environment instructions",
	"code pattern best practice convention and design",
	"error message workaround resolution and explanation",
}

// noiseProtos are generic descriptions of low-signal content.
var noiseProtos = []string{
	"greeting acknowledgment helpful response sure happy",
	"let me know if you need anything else feel free",
	"i will help you with that certainly of course",
}

// ContentScorer scores chunks by their semantic proximity to high-value
// vs. low-value knowledge, using only the embedding model already in use.
// It is safe for concurrent use after construction.
type ContentScorer struct {
	qualityVecs [][]float32
	noiseVecs   [][]float32
}

// NewContentScorer embeds the quality and noise prototypes. Should be called
// once during daemon startup. On error, returns nil — callers should treat a
// nil scorer as "scoring disabled" and continue without content scoring.
func NewContentScorer(ctx context.Context, emb embedding.Embedder) (*ContentScorer, error) {
	qualityVecs, err := emb.EmbedBatch(ctx, qualityProtos)
	if err != nil {
		return nil, err
	}
	noiseVecs, err := emb.EmbedBatch(ctx, noiseProtos)
	if err != nil {
		return nil, err
	}
	return &ContentScorer{qualityVecs: qualityVecs, noiseVecs: noiseVecs}, nil
}

// Score returns a content quality score in [0.0, 1.0] for the given embedding
// vector. A score near 1.0 means the chunk is semantically close to high-value
// knowledge prototypes; near 0.0 means it resembles noise.
//
// Uses ratio normalization: score = avgQualitySim / (avgQualitySim + avgNoiseSim)
// so the result is always in (0, 1) and independent of absolute similarity magnitudes.
func (cs *ContentScorer) Score(vec []float32) float64 {
	if cs == nil || len(vec) == 0 {
		return 0.5 // neutral default when scorer unavailable
	}

	var qualitySum, noiseSum float64
	for _, q := range cs.qualityVecs {
		qualitySum += cosineSim(vec, q)
	}
	for _, n := range cs.noiseVecs {
		noiseSum += cosineSim(vec, n)
	}

	avgQuality := qualitySum / float64(len(cs.qualityVecs))
	avgNoise := noiseSum / float64(len(cs.noiseVecs))

	denom := avgQuality + avgNoise
	if denom <= 0 {
		return 0.5
	}
	score := avgQuality / denom
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// ContentScaleHalfLife returns the effective decay half-life for a chunk
// based on its content score. High-quality chunks keep the full configured
// half-life; low-quality chunks get a shorter one, falling below the prune
// threshold much sooner.
//
// The minimum effective half-life is 7 days, regardless of config.
// At content_score=0 and a 90-day base: ~7-day half-life → pruned in ~33 days.
// At content_score=1 and a 90-day base: full 90-day half-life.
func ContentScaleHalfLife(halfLife float64, contentScore float64) float64 {
	if contentScore < 0 {
		contentScore = 0
	}
	if contentScore > 1 {
		contentScore = 1
	}
	minHalfLife := float64(7 * 24 * time.Hour)
	if halfLife <= minHalfLife {
		return halfLife
	}
	return minHalfLife + contentScore*(halfLife-minHalfLife)
}

func cosineSim(a, b []float32) float64 {
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
