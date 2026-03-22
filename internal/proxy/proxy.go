package proxy

import (
	"fmt"
	"log"
	"net/http"

	"github.com/kindling-sh/memoryd/internal/config"
	"github.com/kindling-sh/memoryd/internal/embedding"
	"github.com/kindling-sh/memoryd/internal/ingest"
	"github.com/kindling-sh/memoryd/internal/pipeline"
	"github.com/kindling-sh/memoryd/internal/quality"
	"github.com/kindling-sh/memoryd/internal/store"
)

// Server is the memoryd HTTP proxy.
type Server struct {
	httpServer *http.Server
}

type serverOpts struct {
	store       store.Store
	sourceStore store.SourceStore
	ingester    *ingest.Ingester
	quality     *quality.Tracker
	embedder    embedding.Embedder
}

// ServerOption configures the server.
type ServerOption func(*serverOpts)

// WithStore enables the web dashboard.
func WithStore(st store.Store) ServerOption {
	return func(o *serverOpts) { o.store = st }
}

// WithSourceStore enables source ingestion API endpoints.
func WithSourceStore(ss store.SourceStore) ServerOption {
	return func(o *serverOpts) { o.sourceStore = ss }
}

// WithIngester sets the source ingester.
func WithIngester(ing *ingest.Ingester) ServerOption {
	return func(o *serverOpts) { o.ingester = ing }
}

// WithQuality sets the quality tracker.
func WithQuality(qt *quality.Tracker) ServerOption {
	return func(o *serverOpts) { o.quality = qt }
}

// WithEmbedder sets the embedder for re-embedding edited content.
func WithEmbedder(emb embedding.Embedder) ServerOption {
	return func(o *serverOpts) { o.embedder = emb }
}

// NewServer wires up all endpoints and returns a ready-to-start server.
func NewServer(cfg *config.Config, read *pipeline.ReadPipeline, write *pipeline.WritePipeline, opts ...ServerOption) *Server {
	var so serverOpts
	for _, o := range opts {
		o(&so)
	}

	mux := http.NewServeMux()

	client := &http.Client{} // no timeout -- streaming responses can be long-lived

	mux.Handle("/v1/messages", newAnthropicHandler(cfg.UpstreamAnthropicURL, write, client, cfg.ProxyWriteEnabled()))
	mux.Handle("/v1/chat/completions", newOpenAIHandler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	if so.store != nil {
		registerDashboard(mux, so.store, so.sourceStore, so.quality)
		registerAPI(mux, so.store, read, write, so.embedder)
	}

	if so.sourceStore != nil && so.ingester != nil {
		registerSourceAPI(mux, so.sourceStore, so.store, so.ingester, so.quality)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

func (s *Server) Start() error {
	log.Printf("memoryd listening on %s", s.httpServer.Addr)
	log.Printf("  export ANTHROPIC_BASE_URL=http://%s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.httpServer.Close()
}
