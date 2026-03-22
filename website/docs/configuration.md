---
sidebar_position: 6
title: Configuration
---

# Configuration

memoryd reads configuration from `~/.memoryd/config.yaml`. All fields have sensible defaults — only `mongodb_atlas_uri` is required.

## Full reference

```yaml
# Required — MongoDB connection string
mongodb_atlas_uri: "mongodb+srv://user:pass@cluster0.mongodb.net/?retryWrites=true"

# Proxy port (default: 7432)
port: 7432

# MongoDB database name (default: "memoryd")
mongodb_database: "memoryd"

# Embedding model path (default: ~/.memoryd/models/voyage-4-nano.gguf)
# Downloaded automatically on first run
model_path: "~/.memoryd/models/voyage-4-nano.gguf"

# Embedding dimensions (default: 1024)
# Must match the model and your Atlas vector index
embedding_dim: 1024

# Number of memories to retrieve per query (default: 5)
retrieval_top_k: 5

# Max tokens of context to inject (default: 2048)
# ~8KB of memory content per request
retrieval_max_tokens: 2048

# Upstream API URL (default: https://api.anthropic.com)
upstream_anthropic_url: "https://api.anthropic.com"

# Enable Atlas hybrid search features (default: false)
# Set to true when using a MongoDB Atlas cluster (not local)
atlas_mode: false

# Steward configuration — memory quality maintenance
steward:
  # Minutes between steward sweeps (default: 60)
  interval_minutes: 60

  # Quality score below which memories are eligible for pruning (default: 0.1)
  prune_threshold: 0.1

  # Hours a memory must exist before pruning is considered (default: 24)
  grace_period_hours: 24

  # Days for quality score half-life decay (default: 7)
  decay_half_days: 7

  # Cosine similarity threshold for merging near-duplicates (default: 0.88)
  merge_threshold: 0.88

  # Memories processed per steward sweep (default: 500)
  batch_size: 500
```

## Minimal configuration

```yaml
mongodb_atlas_uri: "mongodb://localhost:27017/?directConnection=true"
```

Everything else defaults. This is all you need for local development with Atlas Local (Docker).

## Atlas mode

Set `atlas_mode: true` when connected to a MongoDB Atlas cluster. This enables:

- **Hybrid search** — vector + full-text Lucene with RRF fusion and MMR diversification
- **Quality pre-filtering** — search results filtered by quality score at the database level
- **Source-scoped search** — filter results by source using regex

Without `atlas_mode`, memoryd uses plain vector search, which works well for individual use but doesn't scale as effectively for [team deployments](team-knowledge-hub).

## Steward tuning

The steward defaults are calibrated for a typical solo developer workflow. For teams or high-volume stores, consider:

| Scenario | Adjustment |
|---|---|
| Large team, many contributors | Lower `interval_minutes` (30), increase `batch_size` (1000) |
| Want to keep memories longer | Increase `decay_half_days` (14–30) |
| Aggressive dedup | Lower `merge_threshold` (0.85) |
| Conservative pruning | Lower `prune_threshold` (0.05), increase `grace_period_hours` (72) |
| High-volume source ingestion | Increase `batch_size` (2000), lower `interval_minutes` (30) |

## Environment variables

The only environment variable memoryd uses indirectly is:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:7432
```

This isn't a memoryd config — it tells your agent to route through the memoryd proxy. memoryd itself reads everything from `config.yaml`.
