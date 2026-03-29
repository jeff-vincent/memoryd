# memoryd Comprehensive Benchmark — 2026-03-27 21:53:55

**Dataset:** `nlile/misc-merged-claude-code-traces-v1`  
**Rows processed:** 100  
**Wall time:** 5s  
**Throughput:** 21.4 rows/sec  
**Memories before:** 175  
**Memories after:** 178 (delta: +3)  
**Source tag:** `benchmark-1774673630128568000`  

---

## 1. Local Classification Distribution

Each row classified locally before ingestion (same heuristics as the HF pipeline optimiser).

| Class | Count | % |
|-------|-------|---|
| noise | 4 | 4.0% |
| explanation | 1 | 1.0% |
| substantive | 95 | 95.0% |

## 2. Pipeline Stage Distribution

| Stage | Count | % | Description |
|-------|-------|---|-------------|
| stored | 3 | 3.0% | Distilled entry reached MongoDB |
| noise_filtered | 97 | 97.0% | Write pipeline noise gate (no synthesizer path) |

**Store rate:** 3 / 100 (3.0%)  
**Total filtered:** 97 / 100 (97.0%)  

## 3. Cross-Tabulation: Classification × Pipeline Stage

How did each classification category flow through the pipeline?

| Classification | stored | noise_filtered | Total |
|---|---|---|---|
| **noise** | 0 (0%) | 4 (100%) | 4 |
| **explanation** | 0 (0%) | 1 (100%) | 1 |
| **substantive** | 3 (3%) | 92 (97%) | 95 |

**Key metric — explanation leak rate:** 0 / 1 explanation rows stored (0.0%). Lower is better — these carry no durable knowledge.

**Key metric — substantive false-positive rate:** 92 / 95 substantive rows filtered (96.8%). Lower is better — these should be stored.

## 4. Content Length Analysis

### Assistant response length × pipeline outcome

| Length bucket | Total | Stored | Filtered | Store rate |
|-------------|-------|--------|----------|------------|
| < 100 chars | 22 | 2 | 20 | 9% |
| 100–500 chars | 5 | 1 | 4 | 20% |
| 1K–5K chars | 1 | 0 | 1 | 0% |
| 10K–32K chars | 1 | 0 | 1 | 0% |

### Synthesis compression (stored rows only)

_No stored rows with synthesis data. Synthesizer may not be active._

## 5. Latency Analysis

### Overall

| p50 | p90 | p95 | p99 | mean | min | max |
|-----|-----|-----|-----|------|-----|-----|
| 72 ms | 660 ms | 683 ms | 694 ms | 185 ms | 10 ms | 696 ms |

### By pipeline stage

| Stage | Count | p50 (ms) | p90 (ms) | p99 (ms) | Mean (ms) |
|-------|-------|----------|----------|----------|-----------|
| stored | 3 | 54 | 88 | 88 | 65 |
| noise_filtered | 97 | 72 | 668 | 696 | 189 |

### Latency by response length (stored rows)

| Length bucket | Count | p50 (ms) | p90 (ms) | Mean (ms) |
|-------------|-------|----------|----------|-----------|
| < 100 chars | 2 | 54 | 54 | 54 |
| 100–500 chars | 1 | 88 | 88 | 88 |

## 6. Write Pipeline Detail (stored rows)

Across all 3 stored rows, the write pipeline reported:

| Metric | Count | Meaning |
|--------|-------|---------|
| Chunks stored | 3 | Individual memories inserted into MongoDB |
| Duplicates skipped | 0 | Near-duplicate chunks (cosine >= 0.92) |
| Noise filtered | 0 | Chunks below noise threshold |
| Source extended | 0 | Chunks extending existing source memories |
| Topic merged | 0 | Chunks absorbed into topic groups |

### Dedup accumulation over time

Cumulative duplicate count by row number (every 10% of run).

| Row # | Cumulative dupes |
|-------|-----------------|
| 10 | 8 |
| 20 | 18 |
| 30 | 21 |
| 40 | 21 |
| 50 | 21 |
| 60 | 21 |
| 70 | 21 |
| 80 | 21 |
| 90 | 21 |
| 100 | 21 |

## 7. Search Quality Probes

Queries run after ingestion to test retrieval quality.

| Query | Top Score | Hits | Top Result Preview |
|-------|-----------|------|-------------------|
| how does deduplication work in the memory pipeline | 0.730 | 5 | Search ( memory_search ) — runs the retrieval pipeline with hybrid search, qua… |
| debugging memory retrieval when search returns emp… | 0.766 | 5 | for detailed setup per tool. Step 5: Verify it's working ​ memoryd status # co… |
| MongoDB vector search configuration and indexing | 0.774 | 5 | None of these capabilities (except basic vector search) are available on Communi… |
| security redaction and sensitive data handling | 0.751 | 5 | There it is: **`panic: runtime error: slice bounds out of range [48:46]`** in `r… |
| write pipeline performance and latency bottlenecks | 0.721 | 5 | All done. Here's what shipped: **`internal/config/config.go`** - `NoiseMinLen` d… |
| how to configure memoryd for a team | 0.747 | 5 | Create the search index ​ In the Atlas UI, go to Atlas Search and create a vec… |
| error handling best practices in Go | 0.776 | 5 | 8 errors but no debug output — the `err != nil` path (network error) fires bef… |
| API authentication and token management | 0.789 | 5 | Done. Here's what was built: **New files:** - `internal/config/token.go` — `En… |
| embedding model and vector dimensions | 0.739 | 5 | The field names match — `assistant_response` and `user_prompt` are correct. Le… |
| steward quality scoring and pruning | 0.757 | 5 | 4. Quality feedback ​ When knowledge items are retrieved, memoryd records that… |

**Average top score across all probes:** 0.755

## 8. Rejection Store

- **Total rejections buffered:** 0 / 500 capacity

The rejection store feeds the adaptive noise scorer. After 0 entries,
the scorer has a richer model of what "noise" looks like in real conversations.

## 10. Summary

| Metric | Value |
|--------|-------|
| Rows processed | 100 |
| Store rate | 3.0% |
| Filter rate | 97.0% |
| Explanation leak rate | 0.0% |
| Substantive false-positive rate | 96.8% |
| Memories created | +3 |
| Avg search probe score | 0.755 |
| Median latency | 72 ms |


**Cleanup:** Deleted 3 benchmark memories (source=`benchmark-1774673630128568000`).
