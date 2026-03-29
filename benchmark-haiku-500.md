# memoryd Comprehensive Benchmark — 2026-03-28 23:30:22

**Dataset:** `nlile/misc-merged-claude-code-traces-v1`  
**Rows processed:** 500  
**Wall time:** 29s  
**Throughput:** 17.4 rows/sec  
**Memories before:** 0  
**Memories after:** 16 (delta: +16)  
**Source tag:** `benchmark-1774765793534971000`  

---

## 1. Local Classification Distribution

Each row classified locally before ingestion (same heuristics as the HF pipeline optimiser).

| Class | Count | % |
|-------|-------|---|
| noise | 6 | 1.2% |
| explanation | 1 | 0.2% |
| substantive | 493 | 98.6% |

## 2. Pipeline Stage Distribution

| Stage | Count | % | Description |
|-------|-------|---|-------------|
| stored | 32 | 6.4% | Distilled entry reached MongoDB |
| synthesizer_skip | 40 | 8.0% | LLM judged exchange as no durable value |
| error | 428 | 85.6% | Network / API error |

**Store rate:** 32 / 500 (6.4%)  
**Total filtered:** 40 / 500 (8.0%)  

## 3. Cross-Tabulation: Classification × Pipeline Stage

How did each classification category flow through the pipeline?

| Classification | stored | synthesizer_skip | error | Total |
|---|---|---|---|---|
| **noise** | 0 (0%) | 4 (67%) | 2 (33%) | 6 |
| **explanation** | 0 (0%) | 1 (100%) | 0 (0%) | 1 |
| **substantive** | 32 (6%) | 35 (7%) | 426 (86%) | 493 |

**Key metric — explanation leak rate:** 0 / 1 explanation rows stored (0.0%). Lower is better — these carry no durable knowledge.

**Key metric — substantive false-positive rate:** 461 / 493 substantive rows filtered (93.5%). Lower is better — these should be stored.

## 4. Content Length Analysis

### Assistant response length × pipeline outcome

| Length bucket | Total | Stored | Filtered | Store rate |
|-------------|-------|--------|----------|------------|
| < 100 chars | 26 | 0 | 26 | 0% |
| 100–500 chars | 10 | 0 | 10 | 0% |
| 500–1K chars | 1 | 0 | 1 | 0% |
| 1K–5K chars | 2 | 0 | 2 | 0% |
| 5K–10K chars | 2 | 0 | 2 | 0% |
| 10K–32K chars | 11 | 0 | 11 | 0% |

### Synthesis compression (stored rows only)

- **Samples:** 32 stored rows with synthesized entry
- **Mean entry/response ratio:** 0.01 (entries are 1% the size of original responses)
- **Median ratio:** 0.00
- **Total input:** 5.1 MB → **Total output:** 23.5 KB (99.6% compression)

## 5. Latency Analysis

### Overall

| p50 | p90 | p95 | p99 | mean | min | max |
|-----|-----|-----|-----|------|-----|-----|
| 788 ms | 3553 ms | 4517 ms | 8035 ms | 1726 ms | 439 ms | 8035 ms |

### By pipeline stage

| Stage | Count | p50 (ms) | p90 (ms) | p99 (ms) | Mean (ms) |
|-------|-------|----------|----------|----------|-----------|
| stored | 32 | 2304 | 4517 | 8035 | 2972 |
| synthesizer_skip | 40 | 629 | 788 | 4044 | 729 |

### Latency by response length (stored rows)

| Length bucket | Count | p50 (ms) | p90 (ms) | Mean (ms) |
|-------------|-------|----------|----------|-----------|

## 6. Write Pipeline Detail (stored rows)

Across all 32 stored rows, the write pipeline reported:

| Metric | Count | Meaning |
|--------|-------|---------|
| Chunks stored | 16 | Individual memories inserted into MongoDB |
| Duplicates skipped | 13 | Near-duplicate chunks (cosine >= 0.92) |
| Noise filtered | 0 | Chunks below noise threshold |
| Source extended | 0 | Chunks extending existing source memories |
| Topic merged | 0 | Chunks absorbed into topic groups |

### Dedup accumulation over time

Cumulative duplicate count by row number (every 10% of run).

| Row # | Cumulative dupes |
|-------|-----------------|
| 50 | 4 |
| 100 | 9 |
| 150 | 9 |
| 200 | 10 |
| 250 | 11 |
| 300 | 12 |
| 350 | 12 |
| 400 | 13 |
| 450 | 13 |
| 500 | 13 |

## 7. Search Quality Probes

Queries run after ingestion to test retrieval quality.

| Query | Top Score | Hits | Top Result Preview |
|-------|-----------|------|-------------------|
| how does deduplication work in the memory pipeline | 0.691 | 5 | SKIP The text is procedural narration describing the current state of an existin… |
| debugging memory retrieval when search returns emp… | 0.726 | 5 | SKIP The user's message contains procedural information about reading files and … |
| MongoDB vector search configuration and indexing | 0.720 | 5 | SKIP The user's message contains procedural information about reading files and … |
| security redaction and sensitive data handling | 0.743 | 5 | SKIP The user's message contains procedural information about reading files and … |
| write pipeline performance and latency bottlenecks | 0.712 | 5 | SKIP The user's message contains procedural information about reading files and … |
| how to configure memoryd for a team | 0.670 | 5 | SKIP The user asked me to function as a memory curator, but this is the middle o… |
| error handling best practices in Go | 0.743 | 5 | SKIP The user's message contains procedural information about reading files and … |
| API authentication and token management | 0.703 | 5 | SKIP The user's message contains procedural information about reading files and … |
| embedding model and vector dimensions | 0.731 | 5 | SKIP The text is procedural narration showing file content exploration without c… |
| steward quality scoring and pruning | 0.698 | 5 | SKIP The user's message contains procedural information about reading files and … |

**Average top score across all probes:** 0.714

## 8. Rejection Store

- **Total rejections buffered:** 40 / 500 capacity

| Stage | Count |
|-------|-------|
| synthesizer | 40 |

The rejection store feeds the adaptive noise scorer. After 40 entries,
the scorer has a richer model of what "noise" looks like in real conversations.

## 9. Errors

**Total errors:** 428 / 500 (85.6%)

| Error | Count |
|-------|-------|
| HTTP 500: {
  "error": "synthesis error: synthesizer: API error 429: {\"type\":\"error\",\"error\":{… | 428 |

## 10. Summary

| Metric | Value |
|--------|-------|
| Rows processed | 500 |
| Store rate | 6.4% |
| Filter rate | 8.0% |
| Explanation leak rate | 0.0% |
| Substantive false-positive rate | 93.5% |
| Memories created | +16 |
| Avg search probe score | 0.714 |
| Median latency | 788 ms |
| Median synthesis compression | 100% |


**Cleanup:** Benchmark memories retained (source=`benchmark-1774765793534971000`). Delete manually if desired.
