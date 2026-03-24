package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/memory-daemon/memoryd/internal/config"
	"github.com/memory-daemon/memoryd/internal/store"
)

type mockEmbedder struct {
	dim     int
	calls   int
	failOn  int
	lastTxt string
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	m.calls++
	m.lastTxt = text
	if m.failOn > 0 && m.calls == m.failOn {
		return nil, context.DeadlineExceeded
	}
	vec := make([]float32, m.dim)
	for i := range vec {
		vec[i] = float32(i) * 0.001
	}
	return vec, nil
}

func (m *mockEmbedder) Dim() int     { return m.dim }
func (m *mockEmbedder) Close() error { return nil }

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := m.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		vecs[i] = v
	}
	return vecs, nil
}

type mockStore struct {
	memories  []store.Memory
	inserted  []store.Memory
	searchErr error
}

func (m *mockStore) VectorSearch(_ context.Context, _ []float32, topK int) ([]store.Memory, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	limit := topK
	if limit > len(m.memories) {
		limit = len(m.memories)
	}
	return m.memories[:limit], nil
}

func (m *mockStore) Insert(_ context.Context, mem store.Memory) error {
	m.inserted = append(m.inserted, mem)
	return nil
}

func (m *mockStore) Delete(_ context.Context, id string) error { return nil }
func (m *mockStore) List(_ context.Context, _ string, _ int) ([]store.Memory, error) {
	return m.memories, nil
}
func (m *mockStore) DeleteAll(_ context.Context) error                        { return nil }
func (m *mockStore) CountBySource(_ context.Context, _ string) (int64, error) { return 0, nil }
func (m *mockStore) UpdateContent(_ context.Context, _ string, _ string, _ []float32) error {
	return nil
}
func (m *mockStore) ListBySource(_ context.Context, _ string, _ int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

func TestReadPipeline_Retrieve_WithMatches(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{
		memories: []store.Memory{
			{Content: "Go uses goroutines for concurrency.", Source: "claude-code", Score: 0.92},
			{Content: "MongoDB Atlas supports vector search.", Source: "manual", Score: 0.85},
		},
	}
	cfg := &config.Config{RetrievalTopK: 5, RetrievalMaxTokens: 2048}

	rp := NewReadPipeline(emb, st, cfg)
	result, err := rp.Retrieve(context.Background(), "how does Go handle concurrency?")
	if err != nil {
		t.Fatalf("Retrieve() error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(result, "goroutines") {
		t.Error("expected context to contain goroutines")
	}
	if !strings.Contains(result, "vector search") {
		t.Error("expected context to contain vector search")
	}
	if !strings.Contains(result, "<retrieved_context>") {
		t.Error("expected XML tags in formatted context")
	}
	if emb.calls != 1 {
		t.Errorf("embedder called %d times, want 1", emb.calls)
	}
}

func TestReadPipeline_Retrieve_NoMatches(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{memories: nil}
	cfg := &config.Config{RetrievalTopK: 5, RetrievalMaxTokens: 2048}

	rp := NewReadPipeline(emb, st, cfg)
	result, err := rp.Retrieve(context.Background(), "obscure query")
	if err != nil {
		t.Fatalf("Retrieve() error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestReadPipeline_Retrieve_EmbeddingError(t *testing.T) {
	emb := &mockEmbedder{dim: 512, failOn: 1}
	st := &mockStore{}
	cfg := &config.Config{RetrievalTopK: 5, RetrievalMaxTokens: 2048}

	rp := NewReadPipeline(emb, st, cfg)
	_, err := rp.Retrieve(context.Background(), "test")
	if err == nil {
		t.Error("expected error when embedder fails")
	}
}

func TestReadPipeline_Retrieve_StoreError(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{searchErr: context.DeadlineExceeded}
	cfg := &config.Config{RetrievalTopK: 5, RetrievalMaxTokens: 2048}

	rp := NewReadPipeline(emb, st, cfg)
	_, err := rp.Retrieve(context.Background(), "test")
	if err == nil {
		t.Error("expected error when store fails")
	}
}

func TestWritePipeline_Process_SingleChunk(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	wp := NewWritePipeline(emb, st)
	wp.Process("Short text to store.", "test-source", map[string]any{"session": "s1"})

	if len(st.inserted) != 1 {
		t.Fatalf("expected 1 insertion, got %d", len(st.inserted))
	}
	if st.inserted[0].Content != "Short text to store." {
		t.Errorf("content = %q", st.inserted[0].Content)
	}
	if st.inserted[0].Source != "test-source" {
		t.Errorf("source = %q, want test-source", st.inserted[0].Source)
	}
	if len(st.inserted[0].Embedding) != 512 {
		t.Errorf("embedding dim = %d, want 512", len(st.inserted[0].Embedding))
	}
}

func TestWritePipeline_Process_MultipleChunks(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString(strings.Repeat("word ", 30))
		sb.WriteString("\n\n")
	}

	wp := NewWritePipeline(emb, st)
	wp.Process(sb.String(), "test", nil)

	if len(st.inserted) < 2 {
		t.Errorf("expected multiple chunks stored, got %d", len(st.inserted))
	}
	for i, m := range st.inserted {
		if len(m.Embedding) != 512 {
			t.Errorf("chunk %d: embedding dim = %d, want 512", i, len(m.Embedding))
		}
	}
}

func TestWritePipeline_Process_EmbeddingFailureContinues(t *testing.T) {
	emb := &mockEmbedder{dim: 512, failOn: 1}
	st := &mockStore{}

	text := "Paragraph one.\n\nParagraph two."
	wp := NewWritePipeline(emb, st)
	wp.Process(text, "test", nil)

	if emb.calls < 1 {
		t.Error("expected at least 1 embed call")
	}
}

func TestWritePipeline_Process_EmptyText(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	wp := NewWritePipeline(emb, st)
	wp.Process("", "test", nil)

	if len(st.inserted) != 0 {
		t.Errorf("expected 0 insertions for empty text, got %d", len(st.inserted))
	}
	if emb.calls != 0 {
		t.Errorf("expected 0 embed calls for empty text, got %d", emb.calls)
	}
}

// --- Self-filtering tests ---

func TestWritePipeline_DedupSkipsDuplicate(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	// Existing memory with high score — triggers dedup.
	st := &mockStore{
		memories: []store.Memory{
			{Content: "Go uses goroutines.", Score: 0.95},
		},
	}

	wp := NewWritePipeline(emb, st)
	result := wp.ProcessFiltered("Go uses goroutines for concurrency.", "test", nil)

	if result.Duplicates != 1 {
		t.Errorf("expected 1 duplicate, got %d", result.Duplicates)
	}
	if result.Stored != 0 {
		t.Errorf("expected 0 stored, got %d", result.Stored)
	}
	if len(st.inserted) != 0 {
		t.Errorf("expected 0 insertions, got %d", len(st.inserted))
	}
}

func TestWritePipeline_DedupAllowsNovel(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	// Existing memory with low score — not a duplicate.
	st := &mockStore{
		memories: []store.Memory{
			{Content: "Something unrelated.", Score: 0.3},
		},
	}

	wp := NewWritePipeline(emb, st)
	result := wp.ProcessFiltered("MongoDB Atlas supports vector search.", "test", nil)

	if result.Stored != 1 {
		t.Errorf("expected 1 stored, got %d", result.Stored)
	}
	if result.Duplicates != 0 {
		t.Errorf("expected 0 duplicates, got %d", result.Duplicates)
	}
}

func TestWritePipeline_NoiseFilterShortContent(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	wp := NewWritePipeline(emb, st)
	result := wp.ProcessFiltered("hi", "test", nil)

	if result.Filtered != 1 {
		t.Errorf("expected 1 filtered, got %d", result.Filtered)
	}
	if result.Stored != 0 {
		t.Errorf("expected 0 stored, got %d", result.Stored)
	}
	if emb.calls != 0 {
		t.Errorf("expected 0 embed calls for noise, got %d", emb.calls)
	}
}

func TestWritePipeline_NoiseFilterNonAlphanumeric(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	wp := NewWritePipeline(emb, st)
	// Mostly punctuation/symbols — should be filtered.
	result := wp.ProcessFiltered("----====....!!!!@@@@####$$$$", "test", nil)

	if result.Filtered != 1 {
		t.Errorf("expected 1 filtered, got %d", result.Filtered)
	}
}

func TestWritePipeline_ProcessFilteredSummary(t *testing.T) {
	emb := &mockEmbedder{dim: 512}
	st := &mockStore{}

	wp := NewWritePipeline(emb, st)
	result := wp.ProcessFiltered("This is meaningful content worth remembering.", "test", nil)

	summary := result.Summary()
	if summary != "1 stored." {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestWriteResult_SummaryMixed(t *testing.T) {
	r := WriteResult{Stored: 2, Duplicates: 1, Filtered: 1}
	summary := r.Summary()
	if summary != "2 stored, 1 skipped (duplicate), 1 skipped (too short/noisy)." {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestWriteResult_SummaryEmpty(t *testing.T) {
	r := WriteResult{}
	if r.Summary() != "Nothing to store." {
		t.Errorf("unexpected summary: %q", r.Summary())
	}
}

func TestIsNoise(t *testing.T) {
	tests := []struct {
		text  string
		noise bool
	}{
		{"", true},
		{"hi", true},
		{"   short   ", true},
		{"----!!!!@@@@####$$$$%%%%", true},
		{"This is a meaningful sentence with real content.", false},
		{"Go uses goroutines for lightweight concurrency.", false},
	}
	for _, tc := range tests {
		got := isNoise(tc.text)
		if got != tc.noise {
			t.Errorf("isNoise(%q) = %v, want %v", tc.text, got, tc.noise)
		}
	}
}
