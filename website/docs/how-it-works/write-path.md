---
sidebar_position: 2
title: The Write Path
---

# The Write Path

Every response from the upstream model is captured and processed into memory — asynchronously, with zero impact on response latency.

## Flow

```
Response text → Chunk → Filter noise → Redact secrets → Batch embed → Dedup → Store
```

### 1. Chunk

Raw text is split into memory-sized pieces using a paragraph-aware chunker:

- Split on double newlines (paragraph boundaries)
- If a paragraph exceeds **512 tokens** (~2048 chars), split further on sentence boundaries (`. `)
- Accumulate into chunks up to the token budget, then flush

This preserves logical boundaries. A function definition stays together. A list of steps stays together. Only oversized blocks get split.

### 2. Filter noise

Each chunk passes through a noise gate:

- **Too short** — under 20 characters → skip
- **Not text** — less than 40% alphanumeric characters (catches binary data, ASCII art, raw hex) → skip

Chunks that survive are passed through the **redaction engine** before embedding.

### 3. Redact secrets

Before anything is stored, 13 regex patterns scrub sensitive content:

| Pattern | Replacement |
|---|---|
| AWS access keys (`AKIA...`) | `[REDACTED:AWS_KEY]` |
| AWS secret keys | `[REDACTED:AWS_SECRET]` |
| GitHub tokens (`ghp_`, `gho_`, PATs) | `[REDACTED:GITHUB_TOKEN]` |
| Slack tokens (`xox...`) | `[REDACTED:SLACK_TOKEN]` |
| Stripe keys (`sk_live_...`) | `[REDACTED:STRIPE_KEY]` |
| Private key blocks (`-----BEGIN...`) | `[REDACTED:PRIVATE_KEY]` |
| Connection strings (passwords in URIs) | `[REDACTED:CONNECTION_STRING]` |
| JWTs (`eyJ...`) | `[REDACTED:JWT]` |
| SSH keys | `[REDACTED:SSH_KEY]` |
| Bearer tokens | `[REDACTED]` |
| Generic API keys / secrets | `[REDACTED]` |
| Passwords in key-value pairs | `[REDACTED]` |

A second pass scans each line for key-value patterns containing sensitive keywords (`password`, `secret`, `token`, `api_key`, `credential`, etc.) and redacts their values.

### 4. Batch embed

Valid chunks are embedded in a single batch call to voyage-4-nano, producing one 1024-dimensional vector per chunk. Batching amortizes the overhead of model inference.

### 5. Deduplicate

Each vector is compared against its nearest neighbor in the store:

| Similarity | Action |
|---|---|
| **≥ 0.92** | **Duplicate** — skip entirely |
| **≥ 0.75** (from a source) | **Source extension** — store with metadata linking it to the original: `extends_source`, `extends_memory`, `extends_score` |
| **< 0.75** | **Novel** — store normally |

Source extension tracking builds a knowledge graph: when agent-generated insights expand on ingested documentation, the link is preserved.

### 6. Store

Each surviving chunk becomes a `Memory` document in MongoDB:

```
content, embedding, source, created_at, metadata
```

## Result accounting

Every `ProcessFiltered` call returns a tally:

```
stored: 3 | duplicates: 1 | filtered: 2 | extended: 1
```

This feeds into the dashboard and quality stats for visibility into what's actually accumulating.

## Why async?

The write path runs in a goroutine. The proxy returns the response to the agent immediately and processes memory in the background. This is critical — memory capture should never add latency to the developer experience.
