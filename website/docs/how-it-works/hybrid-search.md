---
sidebar_position: 4
title: Hybrid Search
---

# Hybrid Search

When connected to a MongoDB Atlas cluster, memoryd upgrades from plain vector search to a hybrid pipeline that combines semantic similarity, keyword matching, and diversity optimization.

## Two modes, one interface

memoryd detects at runtime whether the store supports the `HybridSearcher` interface:

| Mode | Store | Search strategy |
|---|---|---|
| **Local** | MongoDB standalone / Atlas Local | Vector search only (cosine similarity) |
| **Atlas** | MongoDB Atlas cluster | Hybrid: vector + text + RRF + MMR |

No configuration change required. Connect to Atlas, get hybrid search automatically.

## The hybrid pipeline

```
Query → [Vector Search] + [Text Search] → RRF Fusion → MMR Diversification → Top-K
```

### Step 1: Vector search (semantic)

The query embedding is compared against stored memory embeddings using Atlas Vector Search:

- Index: `vector_index` on the `embedding` field
- Candidates: `topK × 20` (oversamples, then trims)
- Pre-filter: `quality_score ≥ 0.05 OR quality_score == 0` (keeps new + quality memories, drops garbage)
- Optional source filter via regex

### Step 2: Text search (lexical)

A parallel Lucene full-text search runs against the `content` field using the `text_index`. This catches exact keyword matches that embedding similarity might miss — acronyms, error codes, specific class names.

### Step 3: Reciprocal Rank Fusion (RRF)

The two result lists are fused using RRF with smoothing constant $k = 60$:

$$
\text{score}(d) = \sum_{L \in \{vector, text\}} \frac{1}{\text{rank}_L(d) + k + 1}
$$

A memory ranked #1 in vector search and #3 in text search gets a combined score of $\frac{1}{62} + \frac{1}{64}$. RRF is rank-based, not score-based, so it's robust to the different score scales of vector similarity and Lucene relevance.

### Step 4: Maximal Marginal Relevance (MMR)

The fused results are re-ranked to maximize diversity while preserving relevance:

$$
\text{MMR}(d) = \lambda \cdot \text{relevance}(d) - (1 - \lambda) \cdot \max_{d_j \in S} \text{sim}(d, d_j)
$$

Where:
- $\lambda = 0.7$ (default) — 70% weight on relevance, 30% on diversity
- $S$ = already-selected results
- $\text{sim}$ = cosine similarity between embeddings

MMR greedily selects the next result that is both relevant to the query and different from what's already been selected. This prevents the top-K from being five slight variations of the same memory.

## Why hybrid?

Pure vector search is good at "what is this about?" but weak on exact matches. If a developer asks about `ERR_CONN_REFUSED`, vector search finds memories about connection errors in general — but text search finds the one that mentions that exact error code.

RRF lets both signals contribute without requiring calibration. MMR then ensures the final results cover different aspects of the query instead of clustering around a single concept.

## Fetch factor

Both search phases use a fetch factor of `max(topK × 4, 20)` candidates. This ensures enough diversity in the candidate pool for MMR to work with, even for small top-K values.
