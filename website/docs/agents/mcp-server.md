---
sidebar_position: 1
title: MCP Server
---

# MCP Server

memoryd exposes its full memory interface over the **Model Context Protocol (MCP)** — a standard JSON-RPC protocol that any compatible agent can speak. This is the primary integration point for agents beyond the proxy.

## The key idea

The MCP server makes memoryd's knowledge store accessible to **any agentic tool** that supports MCP. The store is the product, not the agent that populates it.

Today, the proxy captures knowledge from Claude Code sessions. But any MCP-connected agent — Cursor, Windsurf, a custom LangChain pipeline, an internal tool — can read from (and write to) the same store. The knowledge accumulates from all sources and benefits all consumers.

## Setup

Add memoryd as an MCP server in your agent's configuration:

```json
{
  "mcpServers": {
    "memoryd": {
      "command": "memoryd",
      "args": ["mcp"]
    }
  }
}
```

memoryd communicates over **stdio** (line-delimited JSON-RPC), implementing MCP protocol version `2024-11-05`.

## Tools

The MCP server exposes 8 tools:

### Memory operations

| Tool | Parameters | Description |
|---|---|---|
| `memory_search` | `query` (required) | Semantic search across all memories. Returns formatted context. |
| `memory_store` | `content` (required), `source` (default: "mcp") | Store new knowledge. Auto-deduplicates, filters noise, redacts secrets. |
| `memory_list` | `query` (optional), `limit` (default: 20) | List memories, optionally filtered by text match. |
| `memory_delete` | `id` (required) | Delete a specific memory by ID. |

### Source ingestion

| Tool | Parameters | Description |
|---|---|---|
| `source_ingest` | `url`, `name` (required), `max_depth` (default: 3), `max_pages` (default: 500) | Crawl a URL and ingest content into the memory store. |
| `source_list` | — | List all ingested sources and their status. |
| `source_remove` | `id` (required) | Remove a source and all its associated memories. |

### Quality

| Tool | Parameters | Description |
|---|---|---|
| `quality_stats` | — | Returns retrieval event count and learning status. |

## How it works

```
Agent  ←→  stdin/stdout (JSON-RPC)  ←→  memoryd mcp  ←→  REST API  ←→  Memory Store
```

The MCP server is a thin adapter. It translates MCP tool calls into REST API requests against the running memoryd daemon (`http://127.0.0.1:{port}`). This means:

- The daemon must be running (`memoryd start`) for MCP tools to work
- MCP writes go through the same pipeline as proxy writes — noise filtering, redaction, dedup
- MCP reads use the same retrieval pipeline — hybrid search when available, quality filtering included

## Read-only usage

An agent can connect via MCP and **only use `memory_search`** — consuming institutional knowledge without writing anything. This is the read-only mode described in [Read-Only Mode](read-only-mode).

This is particularly useful when:
- An agent should benefit from team knowledge but not contribute to it
- You want to try memoryd without committing to full integration
- Security policies restrict what an agent can store
