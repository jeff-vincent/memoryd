---
sidebar_position: 2
title: Proxy Mode
---

# Proxy Mode

The proxy is how memoryd captures knowledge without any developer intervention. Set one environment variable, and every Claude Code session automatically feeds the memory store.

## How it works

memoryd runs an HTTP proxy on `127.0.0.1:7432`. Point your agent's API base URL at it:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:7432
```

From there, the proxy is transparent:

```
Agent → memoryd proxy (7432) → Anthropic API → response
              ↓                        ↓
         capture user msg         capture response
              ↓                        ↓
           write pipeline (async, background)
```

### On the way out

The agent's request passes through unchanged. memoryd extracts the last user message and feeds it to the write pipeline in the background.

### On the way back

The response is streamed back to the agent in real-time. Simultaneously, memoryd buffers the response text and processes it through the write pipeline after the stream completes.

The agent experiences **zero added latency**. Memory capture is fully async.

## Streaming support

memoryd handles both response formats:

- **Synchronous** — reads the full response body, extracts text, forwards to client
- **Server-Sent Events (SSE)** — streams events through to the client in real-time using `http.Flusher`, buffers `text_delta` events, processes the complete text after the stream ends

The SSE handler uses a 1MB scanner buffer and extracts content from `delta.type == "text_delta"` events.

## What gets captured

Both sides of the conversation are captured:

| Source | What's extracted | Write pipeline |
|---|---|---|
| **User message** | Last `user` role message from the request body | Chunked, filtered, redacted, embedded, deduped, stored |
| **Assistant response** | Full text content from sync response or streamed deltas | Same pipeline |

Everything goes through the same write path — noise filtering, secret redaction, deduplication. See [The Write Path](../how-it-works/write-path) for details.

## Context injection

On the **read side**, memoryd intercepts outgoing requests and injects retrieved context into the system prompt. The agent gets relevant memories as part of its instructions without knowing how they got there.

This is the proxy's dual role: capture knowledge on the way back, inject it on the way out.

## Endpoints

The proxy also exposes utility endpoints:

| Endpoint | Purpose |
|---|---|
| `POST /v1/messages` | Anthropic message handler (main proxy path) |
| `POST /v1/chat/completions` | OpenAI-compatible stub |
| `GET /health` | Health check |
| `POST /api/search` | Direct search API |
| `POST /api/store` | Direct store API |
| `GET /api/memories` | List/query memories |
| `GET /api/sources` | Source management |
| `GET /api/quality` | Quality stats |

## When to use proxy vs. MCP

The proxy is ideal when you want **fully automatic** knowledge capture with zero changes to your workflow. Just set the environment variable and work normally.

Use [MCP](mcp-server) when you want explicit agent control over what gets stored and searched, or when integrating with non-Anthropic agents.
