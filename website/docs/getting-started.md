---
sidebar_position: 2
title: Getting Started
---

# Getting Started

## Prerequisites

- **Go 1.26+** — [go.dev/dl](https://go.dev/dl/)
- **llama.cpp** — `brew install llama.cpp` (provides the local embedding server)
- **MongoDB** — either:
  - [MongoDB Atlas](https://www.mongodb.com/atlas) (free tier works, recommended for teams)
  - Atlas Local via Docker (for solo development)

## Install

**Homebrew:**
```bash
brew install memoryd
```

**From source:**
```bash
git clone https://github.com/kindling-sh/memoryd.git
cd memoryd
make build    # → bin/memoryd
```

## Set up MongoDB

### Option A: Atlas (recommended)

1. Create a free-tier cluster at [cloud.mongodb.com](https://cloud.mongodb.com)
2. Create database `memoryd` with collection `memories`
3. Create a vector search index:

```json
{
  "name": "vector_index",
  "type": "vectorSearch",
  "definition": {
    "fields": [{
      "type": "vector",
      "path": "embedding",
      "numDimensions": 1024,
      "similarity": "cosine"
    }]
  }
}
```

4. Copy your connection string

### Option B: Atlas Local (Docker, solo dev)

```bash
docker run -d --name memoryd-mongo -p 27017:27017 mongodb/mongodb-atlas-local:8.0

docker cp scripts/create_index.js memoryd-mongo:/tmp/create_index.js
docker exec memoryd-mongo mongosh memoryd --quiet --file /tmp/create_index.js
```

Connection string: `mongodb://localhost:27017/?directConnection=true`

## Configure

```bash
memoryd start    # creates ~/.memoryd/config.yaml on first run, then exits if URI is empty
```

Edit `~/.memoryd/config.yaml`:

```yaml
mongodb_atlas_uri: "mongodb+srv://user:pass@cluster0.mongodb.net/?retryWrites=true"
```

That's the only required field. Everything else has sensible defaults.

## Run

```bash
# Start the daemon
memoryd start

# Point your coding agent at memoryd
export ANTHROPIC_BASE_URL=http://127.0.0.1:7432
```

The embedding model (~70MB) downloads automatically on first launch. After startup, work normally with Claude Code or any Anthropic-compatible agent. Memory accumulates in the background.

## Verify it's working

```bash
memoryd status        # pings the health endpoint
memoryd search "test" # search stored memories
```

## MCP integration (optional)

To use memoryd as an MCP server with any compatible agent, add to your MCP config:

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

This gives the agent access to `memory_search`, `memory_store`, `source_ingest`, and other tools — without the proxy. See [MCP Server](agents/mcp-server) for details.
