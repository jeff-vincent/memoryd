package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memory-daemon/memoryd/internal/config"
	"github.com/memory-daemon/memoryd/internal/pipeline"
)

func TestExtractLastUserMessage_StringContent(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":"hello world"}]`)
	got := extractLastUserMessageRaw(raw)
	if got != "hello world" {
		t.Errorf("got %q, want hello world", got)
	}
}

func TestExtractLastUserMessage_ArrayContent(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"first part"},{"type":"text","text":"second part"}]}]`)
	got := extractLastUserMessageRaw(raw)
	want := "first part\nsecond part"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractLastUserMessage_MultipleMessages(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":"first"},{"role":"assistant","content":"response"},{"role":"user","content":"most recent"}]`)
	got := extractLastUserMessageRaw(raw)
	if got != "most recent" {
		t.Errorf("got %q, want most recent", got)
	}
}

func TestExtractLastUserMessage_NoMessages(t *testing.T) {
	got := extractLastUserMessageRaw(nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractLastUserMessage_EmptyMessages(t *testing.T) {
	raw := json.RawMessage(`[]`)
	got := extractLastUserMessageRaw(raw)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractLastUserMessage_OnlyAssistant(t *testing.T) {
	raw := json.RawMessage(`[{"role":"assistant","content":"hello"}]`)
	got := extractLastUserMessageRaw(raw)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResponseText_ValidResponse(t *testing.T) {
	resp := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "Hello "},
			map[string]any{"type": "text", "text": "world!"},
		},
	}
	body, _ := json.Marshal(resp)
	got := extractResponseText(body)
	want := "Hello \nworld!"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractResponseText_EmptyContent(t *testing.T) {
	resp := map[string]any{"content": []any{}}
	body, _ := json.Marshal(resp)
	got := extractResponseText(body)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractResponseText_InvalidJSON(t *testing.T) {
	got := extractResponseText([]byte("not json"))
	if got != "" {
		t.Errorf("got %q, want empty for invalid JSON", got)
	}
}

func TestExtractResponseText_NoContent(t *testing.T) {
	resp := map[string]any{"id": "msg123"}
	body, _ := json.Marshal(resp)
	got := extractResponseText(body)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractTextDelta_Valid(t *testing.T) {
	var buf strings.Builder
	data := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello "}}`
	extractTextDelta(data, &buf)
	if buf.String() != "hello " {
		t.Errorf("buf = %q, want hello ", buf.String())
	}
}

func TestExtractTextDelta_NotTextDelta(t *testing.T) {
	var buf strings.Builder
	data := `{"type":"content_block_start","delta":{"type":"start"}}`
	extractTextDelta(data, &buf)
	if buf.String() != "" {
		t.Errorf("buf should be empty for non-text_delta, got %q", buf.String())
	}
}

func TestExtractTextDelta_InvalidJSON(t *testing.T) {
	var buf strings.Builder
	extractTextDelta("not json", &buf)
	if buf.String() != "" {
		t.Errorf("buf should be empty for invalid JSON, got %q", buf.String())
	}
}

func TestExtractTextDelta_NoDelta(t *testing.T) {
	var buf strings.Builder
	data := `{"type":"message_start"}`
	extractTextDelta(data, &buf)
	if buf.String() != "" {
		t.Errorf("buf should be empty when no delta field, got %q", buf.String())
	}
}

func TestExtractTextDelta_Accumulates(t *testing.T) {
	var buf strings.Builder
	extractTextDelta(`{"delta":{"type":"text_delta","text":"one "}}`, &buf)
	extractTextDelta(`{"delta":{"type":"text_delta","text":"two "}}`, &buf)
	extractTextDelta(`{"delta":{"type":"text_delta","text":"three"}}`, &buf)
	if buf.String() != "one two three" {
		t.Errorf("buf = %q, want one two three", buf.String())
	}
}

func TestCopyHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("X-Custom", "value")

	dst := http.Header{}
	copyHeaders(dst, src)

	if dst.Get("Content-Type") != "application/json" {
		t.Error("Content-Type not copied")
	}
	if dst.Get("X-Custom") != "value" {
		t.Error("X-Custom not copied")
	}
}

func TestOpenAIHandler_Returns501(t *testing.T) {
	handler := newOpenAIHandler()
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{Port: 0}
	read := pipeline.NewReadPipeline(nil, nil, cfg)
	write := pipeline.NewWritePipeline(nil, nil)
	srv := NewServer(cfg, read, write)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Error("expected ok in health response")
	}
}

func TestAnthropicHandler_SyncRoundTrip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("x-api-key not forwarded")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("anthropic-version not forwarded")
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_test",
			"type": "message",
			"content": []any{
				map[string]any{"type": "text", "text": "Test response"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	handler := &anthropicHandler{
		upstreamURL: upstream.URL,
		write:       pipeline.NewWritePipeline(nil, nil),
		client:      upstream.Client(),
	}

	reqBody := `{"messages":[{"role":"user","content":"hello"}],"model":"claude-3-5-sonnet-20241022"}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("x-api-key", "test-key")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != "msg_test" {
		t.Errorf("response id = %v, want msg_test", resp["id"])
	}
}

func TestAnthropicHandler_InvalidJSON(t *testing.T) {
	handler := &anthropicHandler{
		upstreamURL: "http://unused",
		client:      &http.Client{},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAnthropicHandler_StreamRoundTrip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_test"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world!"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_stop"}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "%s\n", e)
		}
	}))
	defer upstream.Close()

	handler := &anthropicHandler{
		upstreamURL: upstream.URL,
		write:       pipeline.NewWritePipeline(nil, nil),
		client:      upstream.Client(),
	}

	reqBody := `{"messages":[{"role":"user","content":"hi"}],"stream":true,"model":"claude-3-5-sonnet-20241022"}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("content-type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Hello ") {
		t.Error("expected Hello in streamed response")
	}
	if !strings.Contains(body, "world!") {
		t.Error("expected world! in streamed response")
	}
}

func TestAnthropicHandler_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	handler := &anthropicHandler{
		upstreamURL: upstream.URL,
		write:       pipeline.NewWritePipeline(nil, nil),
		client:      upstream.Client(),
	}

	reqBody := `{"messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("content-type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
