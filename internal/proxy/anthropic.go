package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/memory-daemon/memoryd/internal/pipeline"
)

// anthropicHandler transparently proxies /v1/messages, capturing response
// content on the way out for the write pipeline (ingestion). Retrieval is
// handled by the MCP server, not the proxy.
type anthropicHandler struct {
	upstreamURL  string
	write        *pipeline.WritePipeline
	client       *http.Client
	writeEnabled bool // false in mcp or mcp-readonly modes
}

func newAnthropicHandler(upstreamURL string, write *pipeline.WritePipeline, client *http.Client, writeEnabled bool) *anthropicHandler {
	return &anthropicHandler{
		upstreamURL:  upstreamURL,
		write:        write,
		client:       client,
		writeEnabled: writeEnabled,
	}
}

func (h *anthropicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[proxy] incoming %s %s", r.Method, r.URL.Path)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse into raw-message map to extract user text for ingestion.
	var rawReq map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawReq); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// --- Forward to Anthropic unchanged ---
	upReq, err := http.NewRequestWithContext(r.Context(), "POST",
		h.upstreamURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Forward all original headers, skipping hop-by-hop headers and
	// accept-encoding (we need uncompressed responses for streaming/logging).
	for key, values := range r.Header {
		switch strings.ToLower(key) {
		case "connection", "transfer-encoding", "content-length", "host", "accept-encoding":
			continue
		}
		for _, v := range values {
			upReq.Header.Add(key, v)
		}
	}

	upResp, err := h.client.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer upResp.Body.Close()

	// --- Dispatch based on streaming ---
	var isStreaming bool
	if raw, ok := rawReq["stream"]; ok {
		json.Unmarshal(raw, &isStreaming)
	}
	log.Printf("[proxy] streaming=%v, upstream status=%d", isStreaming, upResp.StatusCode)
	if upResp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(upResp.Body)
		// Decompress if gzip for logging
		logBody := errBody
		if upResp.Header.Get("Content-Encoding") == "gzip" {
			if gr, err := gzip.NewReader(bytes.NewReader(errBody)); err == nil {
				logBody, _ = io.ReadAll(gr)
				gr.Close()
			}
		}
		log.Printf("[proxy] upstream error body: %s", string(logBody))
		copyHeaders(w.Header(), upResp.Header)
		w.WriteHeader(upResp.StatusCode)
		w.Write(errBody)
		return
	}
	if isStreaming {
		h.handleStream(w, upResp)
	} else {
		h.handleSync(w, upResp)
	}
}

// handleSync proxies a non-streaming response and kicks off the write path.
func (h *anthropicHandler) handleSync(w http.ResponseWriter, resp *http.Response) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[proxy] error reading upstream body: %v", err)
		return
	}
	w.Write(body)

	if resp.StatusCode == http.StatusOK {
		go func() {
			if h.writeEnabled {
				if text := extractResponseText(body); text != "" {
					h.write.Process(text, "claude-code", nil)
				}
			}
		}()
	}
}

// handleStream proxies SSE events in real time while buffering the full
// response text for the async write path.
func (h *anthropicHandler) handleStream(w http.ResponseWriter, resp *http.Response) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	var responseBuf strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()

		if strings.HasPrefix(line, "data: ") {
			extractTextDelta(line[6:], &responseBuf)
		}
	}

	if text := responseBuf.String(); text != "" {
		log.Printf("[proxy] stream done, writing %d bytes to store", len(text))
		if h.writeEnabled {
			go h.write.Process(text, "claude-code", nil)
		}
	} else {
		log.Printf("[proxy] stream done, no text extracted")
	}
}

// --- helpers ---

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// extractResponseText pulls the assistant text from a non-streaming response.
func extractResponseText(body []byte) string {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	content, ok := resp["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, block := range content {
		if b, ok := block.(map[string]any); ok {
			if t, ok := b["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// extractTextDelta accumulates text from a streaming content_block_delta event.
func extractTextDelta(data string, buf *strings.Builder) {
	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}
	delta, ok := event["delta"].(map[string]any)
	if !ok {
		return
	}
	if t, _ := delta["type"].(string); t == "text_delta" {
		if text, ok := delta["text"].(string); ok {
			buf.WriteString(text)
		}
	}
}
