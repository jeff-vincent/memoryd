---
sidebar_position: 1
title: MCP Server
---

# MCP Server

The MCP (Model Context Protocol) server is how memoryd connects to **any AI coding tool** — Cursor, Windsurf, Cline, Claude Code, custom pipelines, or anything that speaks MCP. It's the universal integration point.

## Why MCP matters for teams

Different team members use different tools. That's fine. MCP is a standard protocol that lets any compatible tool search and store knowledge in the same shared database. The tool doesn't matter — the knowledge store is the product.

Alice uses Claude Code (via proxy), Bob uses Cursor (via MCP), Carol has a custom pipeline (via MCP). They all contribute to and benefit from the same team knowledge.

## Setup

Add memoryd to your tool's MCP configuration:

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

This works with Claude Code, Cursor, Windsurf, Cline, and any tool that supports MCP servers. The memoryd daemon must be running (`memoryd start`) for MCP tools to work.

## Available tools

The MCP server gives AI tools access to 8 capabilities:

### Core knowledge operations

| Tool | What it does |
|---|---|
| `memory_search` | Search the shared knowledge base with a natural language query |
| `memory_store` | Store new knowledge (auto-deduplicated, noise-filtered, secrets scrubbed) |
| `memory_list` | Browse stored knowledge, optionally filtered by text |
| `memory_delete` | Remove a specific knowledge item |

### Source ingestion

| Tool | What it does |
|---|---|
| `source_ingest` | Crawl a URL (wiki, docs site) and add its content to the knowledge base |
| `source_list` | List all ingested sources and their status |
| `source_remove` | Remove a source and all its associated knowledge |

### Quality monitoring

| Tool | What it does |
|---|---|
| `quality_stats` | Check the knowledge base health — retrieval counts, learning status |

## How it fits together

```
Any AI tool ←→ MCP (stdin/stdout) ←→ memoryd ←→ Shared Atlas store
```

MCP writes go through the same [knowledge capture pipeline](../how-it-works/write-path) as proxy writes — noise filtering, secret scrubbing, deduplication. MCP reads use the same [retrieval pipeline](../how-it-works/read-path) — hybrid search, quality filtering, diversity optimization.

The integration method doesn't matter. The knowledge quality is the same regardless of how it enters the store.

## Read-only usage

Any tool can connect via MCP and **only use `memory_search`** — consuming team knowledge without contributing. This is useful for:

- Tools that should read but not write (evaluation, auditing)
- Team members in security-sensitive contexts
- Trial periods before committing to full integration

See [Read-Only Mode](read-only-mode) for more on this pattern.

## For teams using multiple tools

A common team setup:

| Team member | Tool | Connection | Contribution |
|---|---|---|---|
| Alice | Claude Code | Proxy | Automatic — every session captured |
| Bob | Cursor | MCP | Agent decides what to search and store |
| Carol | Custom pipeline | MCP (read-only) | Reads team knowledge, doesn't write |
| Dave | Claude Code + Cursor | Proxy + MCP | Both automatic capture and explicit tools |

All four draw from and contribute to the same knowledge pool.
