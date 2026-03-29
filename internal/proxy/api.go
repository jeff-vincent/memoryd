package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/memory-daemon/memoryd/internal/config"
	"github.com/memory-daemon/memoryd/internal/embedding"
	"github.com/memory-daemon/memoryd/internal/pipeline"
	"github.com/memory-daemon/memoryd/internal/quality"
	"github.com/memory-daemon/memoryd/internal/redact"
	"github.com/memory-daemon/memoryd/internal/rejection"
	"github.com/memory-daemon/memoryd/internal/store"
	"github.com/memory-daemon/memoryd/internal/synthesizer"
)

type apiHandler struct {
	store    store.Store
	multi    *store.MultiStore // non-nil when multi-database is active
	read     *pipeline.ReadPipeline
	write    *pipeline.WritePipeline
	embedder embedding.Embedder
	cfg      *config.Config
	rejLog   *rejection.Store
	synth    *synthesizer.Synthesizer
}

func registerAPI(mux *http.ServeMux, st store.Store, read *pipeline.ReadPipeline, write *pipeline.WritePipeline, emb embedding.Embedder, cfg *config.Config, rejLog *rejection.Store, synth *synthesizer.Synthesizer) {
	h := &apiHandler{store: st, read: read, write: write, embedder: emb, cfg: cfg, rejLog: rejLog, synth: synth}
	if ms, ok := st.(*store.MultiStore); ok {
		h.multi = ms
	}
	mux.HandleFunc("/api/search", h.handleSearch)
	mux.HandleFunc("/api/store", h.handleStore)
	mux.HandleFunc("/api/ingest", h.handleIngest)
	mux.HandleFunc("/api/memories", h.handleMemories)
	mux.HandleFunc("/api/memories/", h.handleMemoryByID)
	mux.HandleFunc("/api/databases", h.handleDatabases)
	mux.HandleFunc("/api/databases/", h.handleDatabaseByName)
	mux.HandleFunc("/api/pipeline", h.handlePipelineConfig)
	mux.HandleFunc("/api/rejections", h.handleRejections)
}

func (a *apiHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Query    string `json:"query"`
		Database string `json:"database,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Query == "" {
		writeJSON(w, 400, map[string]string{"error": "query is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// If a specific database is requested and multi-database is active, search it directly.
	if req.Database != "" && a.multi != nil {
		vec, err := a.embedder.Embed(ctx, req.Query)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "embedding failed: " + err.Error()})
			return
		}
		mems, err := a.multi.SearchTargeted(ctx, req.Database, vec, 5)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		formatted := pipeline.FormatContext(mems, 2048)
		if formatted == "" {
			formatted = "No relevant memories found."
		}
		writeJSON(w, 200, map[string]string{"context": formatted})
		return
	}

	retrieved, memories, err := a.read.RetrieveWithScores(ctx, req.Query)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	scores := make([]float64, len(memories))
	for i, m := range memories {
		scores[i] = m.Score
	}
	writeJSON(w, 200, map[string]any{"context": retrieved, "scores": scores})
}

func (a *apiHandler) handleStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Content  string         `json:"content"`
		Source   string         `json:"source,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Content == "" {
		writeJSON(w, 400, map[string]string{"error": "content is required"})
		return
	}
	if req.Source == "" {
		req.Source = "mcp"
	}

	result := a.write.ProcessFiltered(req.Content, req.Source, req.Metadata)

	writeJSON(w, 200, map[string]string{"status": "ok", "summary": result.Summary()})
}

// handleIngest handles POST /api/ingest.
// Runs a user+assistant exchange through the full quality pipeline:
// pre-filter → SynthesizeQA → write. Returns the stage that handled the
// exchange and, if stored, the distilled entry text.
//
// Request: {"user_prompt": "...", "assistant_response": "...", "source": "..."}
// Response: {"stage": "stored|pre_filter|synthesizer_skip|noise_filtered|no_synthesizer", "stored": 0|1, "entry": "..."}
func (a *apiHandler) handleIngest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		UserPrompt        string `json:"user_prompt"`
		AssistantResponse string `json:"assistant_response"`
		Source            string `json:"source,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.AssistantResponse == "" {
		writeJSON(w, 400, map[string]string{"error": "assistant_response is required"})
		return
	}
	if req.Source == "" {
		req.Source = "eval"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Quality gate: requires synthesizer.
	if !a.synth.Available() {
		writeJSON(w, 200, map[string]any{"stage": "no_synthesizer", "stored": 0})
		return
	}

	// Pre-filter (only when user prompt is provided).
	if req.UserPrompt != "" && rejection.QuickFilter(req.UserPrompt, req.AssistantResponse) {
		a.rejLog.Add(rejection.StagePreFilter, req.UserPrompt, req.AssistantResponse)
		writeJSON(w, 200, map[string]any{"stage": "pre_filter", "stored": 0})
		return
	}

	entry, err := a.synth.SynthesizeQA(ctx, req.UserPrompt, req.AssistantResponse)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "synthesis error: " + err.Error()})
		return
	}
	if entry == "" {
		a.rejLog.Add(rejection.StageSynthesizer, req.UserPrompt, req.AssistantResponse)
		writeJSON(w, 200, map[string]any{"stage": "synthesizer_skip", "stored": 0})
		return
	}

	result := a.write.ProcessDirect(entry, req.Source, nil)
	writeJSON(w, 200, map[string]any{
		"stage":   "stored",
		"stored":  result.Stored,
		"entry":   entry,
		"summary": result.Summary(),
	})
}

func (a *apiHandler) handleMemories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := r.URL.Query().Get("q")
	memories, err := a.store.List(ctx, query, 0)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, memories)
}

func (a *apiHandler) handleMemoryByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := r.URL.Path[len("/api/memories/"):]
	if id == "" {
		writeJSON(w, 400, map[string]string{"error": "id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodDelete:
		if err := a.store.Delete(ctx, id); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})

	case http.MethodPut:
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Content == "" {
			writeJSON(w, 400, map[string]string{"error": "content is required"})
			return
		}
		// Redact before storing.
		cleaned := redact.Clean(req.Content)
		// Re-embed the updated content.
		vec, err := a.embedder.Embed(ctx, cleaned)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "embedding failed: " + err.Error()})
			return
		}
		if err := a.store.UpdateContent(ctx, id, cleaned, vec); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})

	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (a *apiHandler) handleDatabases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		if a.multi == nil {
			writeJSON(w, 200, []any{})
			return
		}
		writeJSON(w, 200, a.multi.DatabaseList())

	case http.MethodPost:
		// Add a secondary database.
		if a.multi == nil {
			writeJSON(w, 400, map[string]string{"error": "multi-database not active"})
			return
		}

		var req struct {
			Name     string `json:"name"`
			URI      string `json:"uri"`
			Database string `json:"database"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Name == "" || req.URI == "" || req.Database == "" {
			writeJSON(w, 400, map[string]string{"error": "name, uri, and database are required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// Connect to the new database.
		var ms *store.MongoStore
		var ss store.Store
		if a.cfg.AtlasMode {
			atlas, err := store.NewAtlasStore(ctx, req.URI, req.Database)
			if err != nil {
				writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("connection failed: %v", err)})
				return
			}
			ms = atlas.MongoStore
			ss = atlas
		} else {
			var err error
			ms, err = store.NewMongoStore(ctx, req.URI, req.Database)
			if err != nil {
				writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("connection failed: %v", err)})
				return
			}
			ss = ms
		}

		entry := store.DatabaseEntry{
			Name:        req.Name,
			Database:    req.Database,
			Role:        config.RoleReadOnly,
			URI:         req.URI,
			Store:       ms,
			SearchStore: ss,
			Mongo:       ms,
		}

		if err := a.multi.AddEntry(entry); err != nil {
			ms.Close()
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}

		// Persist to config.
		a.persistDatabases()

		writeJSON(w, 200, map[string]string{"status": "ok", "message": fmt.Sprintf("Database %q added (read-only)", req.Name)})

	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (a *apiHandler) handleDatabaseByName(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	name := strings.TrimPrefix(r.URL.Path, "/api/databases/")
	if name == "" {
		writeJSON(w, 400, map[string]string{"error": "database name is required"})
		return
	}

	if a.multi == nil {
		writeJSON(w, 400, map[string]string{"error": "multi-database not active"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Toggle enabled/disabled.
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}

		if err := a.multi.SetEntryEnabled(name, req.Enabled); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}

		a.persistDatabases()

		state := "enabled"
		if !req.Enabled {
			state = "disabled"
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "message": fmt.Sprintf("Database %q %s", name, state)})

	case http.MethodDelete:
		if err := a.multi.RemoveEntry(name); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}

		a.persistDatabases()

		writeJSON(w, 200, map[string]string{"status": "ok", "message": fmt.Sprintf("Database %q removed", name)})

	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

// handlePipelineConfig handles GET/POST /api/pipeline.
// GET returns the live pipeline and steward configuration.
// POST updates them: pipeline changes apply immediately; steward changes are
// saved to disk and take effect on next restart.
func (a *apiHandler) handlePipelineConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		pCfg := a.write.Config()
		// Return effective proto values (defaults if not customised).
		if len(pCfg.QualityProtos) == 0 {
			pCfg.QualityProtos = quality.DefaultQualityProtos
		}
		if len(pCfg.NoiseProtos) == 0 {
			pCfg.NoiseProtos = quality.DefaultNoiseProtos
		}
		writeJSON(w, 200, map[string]any{
			"pipeline": pCfg,
			"steward":  a.cfg.Steward,
		})

	case http.MethodPost:
		var raw struct {
			Pipeline json.RawMessage `json:"pipeline"`
			Steward  json.RawMessage `json:"steward"`
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}

		scorerReloaded := false
		var notes []string

		if len(raw.Pipeline) > 0 && string(raw.Pipeline) != "null" {
			newCfg := a.write.Config() // start from existing
			if err := json.Unmarshal(raw.Pipeline, &newCfg); err != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid pipeline JSON: " + err.Error()})
				return
			}

			// Validate.
			if newCfg.DedupThreshold <= 0 || newCfg.DedupThreshold > 1 {
				writeJSON(w, 400, map[string]string{"error": "dedup_threshold must be in (0, 1]"})
				return
			}
			if newCfg.TopicBoundaryThreshold < 0 || newCfg.TopicBoundaryThreshold > 1 {
				writeJSON(w, 400, map[string]string{"error": "topic_boundary_threshold must be in [0, 1]"})
				return
			}
			if newCfg.ContentScoreGate < 0 || newCfg.ContentScoreGate > 1 {
				writeJSON(w, 400, map[string]string{"error": "content_score_gate must be in [0, 1]"})
				return
			}
			if newCfg.NoiseMinLen < 1 {
				writeJSON(w, 400, map[string]string{"error": "noise_min_len must be >= 1"})
				return
			}
			if newCfg.MaxGroupChars < 256 {
				writeJSON(w, 400, map[string]string{"error": "max_group_chars must be >= 256"})
				return
			}

			// Check if prototypes changed — reload scorer if so.
			existing := a.write.Config()
			existingQP := existing.QualityProtos
			if len(existingQP) == 0 {
				existingQP = quality.DefaultQualityProtos
			}
			existingNP := existing.NoiseProtos
			if len(existingNP) == 0 {
				existingNP = quality.DefaultNoiseProtos
			}
			newQP := newCfg.QualityProtos
			if len(newQP) == 0 {
				newQP = quality.DefaultQualityProtos
			}
			newNP := newCfg.NoiseProtos
			if len(newNP) == 0 {
				newNP = quality.DefaultNoiseProtos
			}

			protosChanged := !strSlicesEqual(existingQP, newQP) || !strSlicesEqual(existingNP, newNP)
			if protosChanged && a.embedder != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
				defer cancel()
				scorer, err := quality.NewContentScorerWithProtos(ctx, a.embedder, newQP, newNP)
				if err != nil {
					writeJSON(w, 500, map[string]string{"error": "scorer reload failed: " + err.Error()})
					return
				}
				a.write.UpdateScorer(scorer)
				scorerReloaded = true
			}

			a.write.UpdateConfig(newCfg)
			if err := config.SavePipelineConfig(newCfg); err != nil {
				notes = append(notes, "pipeline saved in memory but disk write failed: "+err.Error())
			}
		}

		if len(raw.Steward) > 0 && string(raw.Steward) != "null" {
			sCfg := a.cfg.Steward // start from existing
			if err := json.Unmarshal(raw.Steward, &sCfg); err != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid steward JSON: " + err.Error()})
				return
			}
			a.cfg.Steward = sCfg
			if err := config.SaveStewardConfig(sCfg); err != nil {
				notes = append(notes, "steward disk write failed: "+err.Error())
			} else {
				notes = append(notes, "Steward settings saved — take effect on next restart")
			}
		}

		resp := map[string]any{"status": "ok", "scorer_reloaded": scorerReloaded}
		if len(notes) > 0 {
			resp["note"] = strings.Join(notes, "; ")
		}
		writeJSON(w, 200, resp)

	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

// handleRejections handles GET /api/rejections.
// Returns aggregate stats and a sample of recent rejected exchanges, useful for
// tuning the pre-filter and identifying new procedural patterns.
//
// Query params:
//   - n: sample size (default 20, max 200)
func (a *apiHandler) handleRejections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	n := 20
	if s := r.URL.Query().Get("n"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			n = v
			if n > 200 {
				n = 200
			}
		}
	}

	writeJSON(w, 200, map[string]any{
		"stats":  a.rejLog.Stats(),
		"sample": a.rejLog.Sample(n),
	})
}

// strSlicesEqual returns true if two string slices have identical contents.
func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// persistDatabases saves the current secondary database list to config.
func (a *apiHandler) persistDatabases() {
	if a.multi == nil {
		return
	}
	dbs := a.multi.DatabaseList()
	var cfgDBs []config.DatabaseConfig
	for _, db := range dbs {
		enabled := db.Enabled
		cfgDBs = append(cfgDBs, config.DatabaseConfig{
			Name:     db.Name,
			Database: db.Database,
			Role:     db.Role,
			URI:      db.URI,
			Enabled:  &enabled,
		})
	}
	if err := config.SaveDatabases(cfgDBs); err != nil {
		fmt.Printf("[api] warning: failed to persist database config: %v\n", err)
	}
}
