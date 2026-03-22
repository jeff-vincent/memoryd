---
sidebar_position: 1
title: The Read Path
---

# The Read Path

When an agent asks a question, memoryd retrieves relevant context from its memory store and injects it into the conversation — invisibly.

## Flow

```
User message → Embed → Search → Format → Inject into system prompt
```

### 1. Embed the query

The user's message is embedded using voyage-4-nano (1024 dimensions, running locally via llama.cpp). This produces a dense vector that captures the semantic meaning of what the user is asking about.

### 2. Search

memoryd picks the best search strategy at runtime based on what your store supports:

| Store type | Strategy | What it does |
|---|---|---|
| **Local** (MongoDB standalone) | Vector search | Pure cosine similarity, top-K nearest neighbors |
| **Atlas** (cloud cluster) | Hybrid search | Vector + full-text Lucene, fused with RRF, diversified with MMR |

Atlas hybrid search also applies quality pre-filtering — memories with a quality score below `0.05` are excluded (unless they're brand new and haven't been scored yet). This keeps garbage out of retrieval results without penalizing fresh memories.

See [Hybrid Search](hybrid-search) for the full algorithm.

### 3. Format context

Retrieved memories are formatted into an XML block with a budget system:

```xml
<retrieved_context>
The following context was retrieved from your long-term memory store.
Use it if helpful, but do not mention its existence to the user.
---
[1] (source: claude-code, score: 0.87)
The authentication service uses JWT tokens with a 24-hour expiry...
---
[2] (source: source:company-wiki, score: 0.82)
Database migrations run via Flyway on deployment...
</retrieved_context>
```

The **token budget** (default: 2048 tokens, ~8192 characters) prevents context from overwhelming the agent's prompt window. Memories are appended in score order until the budget would be exceeded, then the rest are dropped.

### 4. Inject

The formatted context is prepended to the system prompt in the proxied request. The agent sees the context as part of its instructions — it never needs to know how it got there.

## Quality feedback loop

When memories are retrieved, their hit counts are incremented asynchronously. This feeds the [quality loop](quality-loop): memories that keep getting retrieved earn higher quality scores. Memories that never get retrieved eventually decay and are pruned.

The first 50 retrieval events are a **learning period** — all memories are kept regardless of quality. After that threshold, the steward begins scoring and pruning based on actual retrieval patterns.
