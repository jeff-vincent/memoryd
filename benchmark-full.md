# memoryd Comprehensive Benchmark — 2026-03-27 22:21:45

**Dataset:** `nlile/misc-merged-claude-code-traces-v1`  
**Rows processed:** 32133  
**Wall time:** 12m53s  
**Throughput:** 41.6 rows/sec  
**Memories before:** 177  
**Memories after:** 1436 (delta: +1259)  
**Source tag:** `benchmark-1774674531466856000`  

---

## 1. Local Classification Distribution

Each row classified locally before ingestion (same heuristics as the HF pipeline optimiser).

| Class | Count | % |
|-------|-------|---|
| noise | 5449 | 17.0% |
| explanation | 535 | 1.7% |
| substantive | 26149 | 81.4% |

## 2. Pipeline Stage Distribution

| Stage | Count | % | Description |
|-------|-------|---|-------------|
| stored | 1259 | 3.9% | Distilled entry reached MongoDB |
| noise_filtered | 28033 | 87.2% | Write pipeline noise gate (no synthesizer path) |
| error | 2841 | 8.8% | Network / API error |

**Store rate:** 1259 / 32133 (3.9%)  
**Total filtered:** 28033 / 32133 (87.2%)  

## 3. Cross-Tabulation: Classification × Pipeline Stage

How did each classification category flow through the pipeline?

| Classification | stored | noise_filtered | error | Total |
|---|---|---|---|---|
| **noise** | 79 (1%) | 2551 (47%) | 2819 (52%) | 5449 |
| **explanation** | 5 (1%) | 530 (99%) | 0 (0%) | 535 |
| **substantive** | 1175 (4%) | 24952 (95%) | 22 (0%) | 26149 |

**Key metric — explanation leak rate:** 5 / 535 explanation rows stored (0.9%). Lower is better — these carry no durable knowledge.

**Key metric — substantive false-positive rate:** 24974 / 26149 substantive rows filtered (95.5%). Lower is better — these should be stored.

## 4. Content Length Analysis

### Assistant response length × pipeline outcome

| Length bucket | Total | Stored | Filtered | Store rate |
|-------------|-------|--------|----------|------------|
| < 100 chars | 14743 | 491 | 14252 | 3% |
| 100–500 chars | 10626 | 655 | 9971 | 6% |
| 500–1K chars | 1575 | 58 | 1517 | 4% |
| 1K–5K chars | 4104 | 55 | 4049 | 1% |
| 5K–10K chars | 204 | 0 | 204 | 0% |
| 10K–32K chars | 129 | 0 | 129 | 0% |

### Synthesis compression (stored rows only)

_No stored rows with synthesis data. Synthesizer may not be active._

## 5. Latency Analysis

### Overall

| p50 | p90 | p95 | p99 | mean | min | max |
|-----|-----|-----|-----|------|-----|-----|
| 160 ms | 444 ms | 560 ms | 876 ms | 210 ms | 0 ms | 3134 ms |

### By pipeline stage

| Stage | Count | p50 (ms) | p90 (ms) | p99 (ms) | Mean (ms) |
|-------|-------|----------|----------|----------|-----------|
| stored | 1259 | 219 | 456 | 835 | 255 |
| noise_filtered | 28033 | 157 | 443 | 879 | 208 |

### Latency by response length (stored rows)

| Length bucket | Count | p50 (ms) | p90 (ms) | Mean (ms) |
|-------------|-------|----------|----------|-----------|
| < 100 chars | 491 | 184 | 398 | 220 |
| 100–500 chars | 655 | 235 | 459 | 257 |
| 500–1K chars | 58 | 331 | 491 | 335 |
| 1K–5K chars | 55 | 398 | 694 | 447 |

## 6. Write Pipeline Detail (stored rows)

Across all 1259 stored rows, the write pipeline reported:

| Metric | Count | Meaning |
|--------|-------|---------|
| Chunks stored | 1259 | Individual memories inserted into MongoDB |
| Duplicates skipped | 0 | Near-duplicate chunks (cosine >= 0.92) |
| Noise filtered | 0 | Chunks below noise threshold |
| Source extended | 0 | Chunks extending existing source memories |
| Topic merged | 0 | Chunks absorbed into topic groups |

### Dedup accumulation over time

Cumulative duplicate count by row number (every 10% of run).

| Row # | Cumulative dupes |
|-------|-----------------|
| 3213 | 99 |
| 6426 | 720 |
| 9639 | 1810 |
| 12852 | 2559 |
| 16065 | 3516 |
| 19278 | 4562 |
| 22491 | 5664 |
| 25704 | 6664 |
| 28917 | 7606 |
| 32130 | 8491 |
| 32133 | 8491 |

## 7. Search Quality Probes

Queries run after ingestion to test retrieval quality.

| Query | Top Score | Hits | Top Result Preview |
|-------|-----------|------|-------------------|
| how does deduplication work in the memory pipeline | 0.730 | 5 | Search ( memory_search ) — runs the retrieval pipeline with hybrid search, qua… |
| debugging memory retrieval when search returns emp… | 0.766 | 5 | for detailed setup per tool. Step 5: Verify it's working ​ memoryd status # co… |
| MongoDB vector search configuration and indexing | 0.775 | 5 | **Q:** "Find the feature matrix implementation and JPMorgan connector configurat… |
| security redaction and sensitive data handling | 0.751 | 5 | There it is: **`panic: runtime error: slice bounds out of range [48:46]`** in `r… |
| write pipeline performance and latency bottlenecks | 0.738 | 5 | **Q:** What is the architecture of the proxy? **A:** The proxy intercepts reques… |
| how to configure memoryd for a team | 0.747 | 5 | Create the search index ​ In the Atlas UI, go to Atlas Search and create a vec… |
| error handling best practices in Go | 0.776 | 5 | 8 errors but no debug output — the `err != nil` path (network error) fires bef… |
| API authentication and token management | 0.828 | 5 | **Q:** test question about auth **A:** The auth system uses JWT tokens stored in… |
| embedding model and vector dimensions | 0.748 | 5 | **Q:** [{"type":"text","text":"Command: ls crates/\nOutput: analytics\napi_model… |
| steward quality scoring and pruning | 0.757 | 5 | 4. Quality feedback ​ When knowledge items are retrieved, memoryd records that… |

**Average top score across all probes:** 0.762

## 8. Rejection Store

- **Total rejections buffered:** 0 / 500 capacity

The rejection store feeds the adaptive noise scorer. After 0 entries,
the scorer has a richer model of what "noise" looks like in real conversations.

## 9. Errors

**Total errors:** 2841 / 32133 (8.8%)

| Error | Count |
|-------|-------|
| Post "http://127.0.0.1:7432/api/ingest": EOF | 22 |
| HTTP 400: {
  "error": "assistant_response is required"
}
 | 2819 |

## 10. Summary

| Metric | Value |
|--------|-------|
| Rows processed | 32133 |
| Store rate | 3.9% |
| Filter rate | 87.2% |
| Explanation leak rate | 0.9% |
| Substantive false-positive rate | 95.5% |
| Memories created | +1259 |
| Avg search probe score | 0.762 |
| Median latency | 160 ms |


**Cleanup:** Benchmark memories retained (source=`benchmark-1774674531466856000`). Delete manually if desired.
