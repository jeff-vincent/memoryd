package steward

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/memory-daemon/memoryd/internal/store"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Config tunes the steward's behavior.
type Config struct {
	// How often the steward runs a sweep (default 1h).
	Interval time.Duration

	// Memories below this quality_score get pruned (default 0.1).
	PruneThreshold float64

	// Minimum age before a memory can be pruned (default 24h).
	PruneGracePeriod time.Duration

	// Score decay half-life: how long until an unretrieved memory loses half its score (default 7d).
	DecayHalfLife time.Duration

	// Cosine similarity above which two memories are candidates for merging (default 0.88).
	MergeThreshold float64

	// Max memories to scan per sweep (default 500). Keeps each cycle bounded.
	BatchSize int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Interval:         1 * time.Hour,
		PruneThreshold:   0.1,
		PruneGracePeriod: 24 * time.Hour,
		DecayHalfLife:    7 * 24 * time.Hour,
		MergeThreshold:   0.88,
		BatchSize:        500,
	}
}

// StewardStore extends the base store with operations the steward needs.
// MongoStore implements this via the new methods we add.
type StewardStore interface {
	store.Store

	// ListOldest returns the oldest memories (by created_at), including embeddings.
	ListOldest(ctx context.Context, limit int) ([]store.Memory, error)

	// UpdateQualityScore sets the quality_score for a memory.
	UpdateQualityScore(ctx context.Context, id primitive.ObjectID, score float64) error
}

// Stats captures what a single sweep accomplished.
type Stats struct {
	Scored  int
	Pruned  int
	Merged  int
	Elapsed time.Duration
}

func (s Stats) String() string {
	return fmt.Sprintf("scored=%d pruned=%d merged=%d elapsed=%s",
		s.Scored, s.Pruned, s.Merged, s.Elapsed.Round(time.Millisecond))
}

// Steward is a long-running background service that maintains memory quality.
type Steward struct {
	cfg   Config
	store StewardStore

	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	lastStats Stats
}

// New creates a steward. Call Start() to begin the background loop.
func New(cfg Config, s StewardStore) *Steward {
	if cfg.Interval == 0 {
		cfg = DefaultConfig()
	}
	return &Steward{
		cfg:   cfg,
		store: s,
	}
}

// Start begins the steward loop in a background goroutine.
// It runs one sweep immediately, then on the configured interval.
func (s *Steward) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop gracefully shuts down the steward and waits for the current sweep to finish.
func (s *Steward) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// LastStats returns the results of the most recent sweep.
func (s *Steward) LastStats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStats
}

func (s *Steward) loop(ctx context.Context) {
	defer s.wg.Done()

	// Run one sweep immediately at startup.
	s.sweep(ctx)

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[steward] shutting down")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *Steward) sweep(ctx context.Context) {
	start := time.Now()
	log.Println("[steward] starting sweep")

	stats := Stats{}

	// Phase 1: Score all memories using decay function.
	scored, err := s.scoreMemories(ctx)
	if err != nil {
		log.Printf("[steward] scoring error: %v", err)
		return
	}
	stats.Scored = scored

	// Phase 2: Prune low-quality memories past grace period.
	pruned, err := s.pruneMemories(ctx)
	if err != nil {
		log.Printf("[steward] pruning error: %v", err)
	}
	stats.Pruned = pruned

	// Phase 3: Merge near-duplicate clusters.
	merged, err := s.mergeNearDuplicates(ctx)
	if err != nil {
		log.Printf("[steward] merge error: %v", err)
	}
	stats.Merged = merged

	stats.Elapsed = time.Since(start)

	s.mu.Lock()
	s.lastStats = stats
	s.mu.Unlock()

	log.Printf("[steward] sweep complete: %s", stats)
}

// scoreMemories computes quality_score for each memory based on:
//   - hit_count: more retrievals = higher base score
//   - recency of last retrieval: decays with half-life
//   - age: older memories get a small boost for surviving this long
//
// Formula: quality_score = baseScore * decayFactor
//
//	where baseScore = log2(hit_count + 1) / log2(maxHits + 1)
//	      decayFactor = 0.5 ^ (timeSinceLastRetrieval / halfLife)
//	Fresh memories with no retrievals start at 0.5 and decay from creation time.
func (s *Steward) scoreMemories(ctx context.Context) (int, error) {
	memories, err := s.store.ListOldest(ctx, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("listing memories: %w", err)
	}

	if len(memories) == 0 {
		return 0, nil
	}

	// Find max hit_count for normalization.
	maxHits := 1
	for _, m := range memories {
		if m.HitCount > maxHits {
			maxHits = m.HitCount
		}
	}

	now := time.Now()
	scored := 0

	for _, m := range memories {
		select {
		case <-ctx.Done():
			return scored, ctx.Err()
		default:
		}

		// Base score from hit frequency (0.0 to 1.0).
		var baseScore float64
		if m.HitCount > 0 {
			baseScore = math.Log2(float64(m.HitCount)+1) / math.Log2(float64(maxHits)+1)
		} else {
			baseScore = 0.5 // benefit of doubt for new memories
		}

		// Decay factor based on time since last useful retrieval.
		lastActive := m.LastRetrieved
		if lastActive.IsZero() {
			lastActive = m.CreatedAt
		}
		elapsed := now.Sub(lastActive)
		decayFactor := math.Pow(0.5, float64(elapsed)/float64(s.cfg.DecayHalfLife))

		score := baseScore * decayFactor

		// Clamp to [0, 1].
		if score > 1.0 {
			score = 1.0
		}
		if score < 0.0 {
			score = 0.0
		}

		if err := s.store.UpdateQualityScore(ctx, m.ID, score); err != nil {
			log.Printf("[steward] failed to score memory %s: %v", m.ID.Hex(), err)
			continue
		}
		scored++
	}

	return scored, nil
}

// pruneMemories deletes low-scoring memories that are older than the grace period.
func (s *Steward) pruneMemories(ctx context.Context) (int, error) {
	memories, err := s.store.ListOldest(ctx, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("listing for prune: %w", err)
	}

	now := time.Now()
	pruned := 0

	for _, m := range memories {
		select {
		case <-ctx.Done():
			return pruned, ctx.Err()
		default:
		}

		// Skip memories still in grace period.
		if now.Sub(m.CreatedAt) < s.cfg.PruneGracePeriod {
			continue
		}

		// Skip memories above the quality threshold.
		if m.QualityScore >= s.cfg.PruneThreshold {
			continue
		}

		// Skip memories that have ever been retrieved — they proved useful once.
		if m.HitCount > 0 {
			continue
		}

		if err := s.store.Delete(ctx, m.ID.Hex()); err != nil {
			log.Printf("[steward] failed to prune %s: %v", m.ID.Hex(), err)
			continue
		}
		log.Printf("[steward] pruned memory %s (score=%.3f, age=%s)",
			m.ID.Hex(), m.QualityScore, now.Sub(m.CreatedAt).Round(time.Hour))
		pruned++
	}

	return pruned, nil
}

// mergeNearDuplicates finds pairs of memories with high cosine similarity
// and keeps the one with the higher hit_count, deleting the other.
// This is the heavy phase — it uses VectorSearch per memory, so we limit scope.
func (s *Steward) mergeNearDuplicates(ctx context.Context) (int, error) {
	// Only scan a subset to keep each sweep bounded.
	scanLimit := s.cfg.BatchSize
	if scanLimit > 200 {
		scanLimit = 200 // cap merge scan since it's O(n) vector searches
	}

	memories, err := s.store.ListOldest(ctx, scanLimit)
	if err != nil {
		return 0, fmt.Errorf("listing for merge: %w", err)
	}

	merged := 0
	deleted := make(map[primitive.ObjectID]bool) // track already-deleted to avoid double-delete

	for _, m := range memories {
		select {
		case <-ctx.Done():
			return merged, ctx.Err()
		default:
		}

		if deleted[m.ID] {
			continue
		}

		// Skip memories without embeddings (shouldn't happen but be safe).
		if len(m.Embedding) == 0 {
			continue
		}

		// Find nearest neighbors.
		neighbors, err := s.store.VectorSearch(ctx, m.Embedding, 5)
		if err != nil {
			continue
		}

		for _, n := range neighbors {
			if n.ID == m.ID || deleted[n.ID] {
				continue
			}

			// Score comes from VectorSearch as cosine similarity.
			if n.Score < s.cfg.MergeThreshold {
				continue
			}

			// Keep the memory with more hits. On tie, keep the older one.
			keep, drop := m, n
			if n.HitCount > m.HitCount || (n.HitCount == m.HitCount && n.CreatedAt.Before(m.CreatedAt)) {
				keep, drop = n, m
			}

			if err := s.store.Delete(ctx, drop.ID.Hex()); err != nil {
				log.Printf("[steward] merge delete failed for %s: %v", drop.ID.Hex(), err)
				continue
			}

			deleted[drop.ID] = true
			merged++
			log.Printf("[steward] merged: kept %s (hits=%d) dropped %s (hits=%d, sim=%.3f)",
				keep.ID.Hex(), keep.HitCount, drop.ID.Hex(), drop.HitCount, n.Score)
		}
	}

	return merged, nil
}
