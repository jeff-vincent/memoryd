package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/memory-daemon/memoryd/internal/config"
	"github.com/memory-daemon/memoryd/internal/embedding"
	"github.com/memory-daemon/memoryd/internal/pipeline"
	"github.com/memory-daemon/memoryd/internal/redact"
	"github.com/memory-daemon/memoryd/internal/store"
)

type apiHandler struct {
	store    store.Store
	multi    *store.MultiStore // non-nil when multi-database is active
	read     *pipeline.ReadPipeline
	write    *pipeline.WritePipeline
	embedder embedding.Embedder
	cfg      *config.Config
}

func registerAPI(mux *http.ServeMux, st store.Store, read *pipeline.ReadPipeline, write *pipeline.WritePipeline, emb embedding.Embedder, cfg *config.Config) {
	h := &apiHandler{store: st, read: read, write: write, embedder: emb, cfg: cfg}
	if ms, ok := st.(*store.MultiStore); ok {
		h.multi = ms
	}
	mux.HandleFunc("/api/search", h.handleSearch)
	mux.HandleFunc("/api/store", h.handleStore)
	mux.HandleFunc("/api/memories", h.handleMemories)
	mux.HandleFunc("/api/memories/", h.handleMemoryByID)
	mux.HandleFunc("/api/databases", h.handleDatabases)
	mux.HandleFunc("/api/databases/", h.handleDatabaseByName)
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
			Name             string `json:"name"`
			URI              string `json:"uri"`
			Database         string `json:"database"`
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
