# memoryd Comprehensive Benchmark — 2026-03-28 10:11:27

**Dataset:** `nlile/misc-merged-claude-code-traces-v1`  
**Rows processed:** 32133  
**Wall time:** 12m3s  
**Throughput:** 44.4 rows/sec  
**Memories before:** 1330  
**Memories after:** 1330 (delta: +0)  
**Source tag:** `benchmark-1774678358353901000`  

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
| error | 32133 | 100.0% | Network / API error |

**Store rate:** 0 / 32133 (0.0%)  
**Total filtered:** 0 / 32133 (0.0%)  

## 3. Cross-Tabulation: Classification × Pipeline Stage

How did each classification category flow through the pipeline?

| Classification | error | Total |
|---|---|---|
| **noise** | 5449 (100%) | 5449 |
| **explanation** | 535 (100%) | 535 |
| **substantive** | 26149 (100%) | 26149 |

**Key metric — explanation leak rate:** 0 / 535 explanation rows stored (0.0%). Lower is better — these carry no durable knowledge.

**Key metric — substantive false-positive rate:** 26149 / 26149 substantive rows filtered (100.0%). Lower is better — these should be stored.

## 4. Content Length Analysis

### Assistant response length × pipeline outcome

| Length bucket | Total | Stored | Filtered | Store rate |
|-------------|-------|--------|----------|------------|
| < 100 chars | 14743 | 0 | 14743 | 0% |
| 100–500 chars | 10626 | 0 | 10626 | 0% |
| 500–1K chars | 1575 | 0 | 1575 | 0% |
| 1K–5K chars | 4104 | 0 | 4104 | 0% |
| 5K–10K chars | 204 | 0 | 204 | 0% |
| 10K–32K chars | 129 | 0 | 129 | 0% |

### Synthesis compression (stored rows only)

_No stored rows with synthesis data. Synthesizer may not be active._

## 5. Latency Analysis

### Latency by response length (stored rows)

| Length bucket | Count | p50 (ms) | p90 (ms) | Mean (ms) |
|-------------|-------|----------|----------|-----------|

## 6. Write Pipeline Detail (stored rows)

Across all 0 stored rows, the write pipeline reported:

| Metric | Count | Meaning |
|--------|-------|---------|
| Chunks stored | 0 | Individual memories inserted into MongoDB |
| Duplicates skipped | 0 | Near-duplicate chunks (cosine >= 0.92) |
| Noise filtered | 0 | Chunks below noise threshold |
| Source extended | 0 | Chunks extending existing source memories |
| Topic merged | 0 | Chunks absorbed into topic groups |

### Dedup accumulation over time

Cumulative duplicate count by row number (every 10% of run).

| Row # | Cumulative dupes |
|-------|-----------------|
| 3213 | 0 |
| 6426 | 0 |
| 9639 | 0 |
| 12852 | 0 |
| 16065 | 0 |
| 19278 | 0 |
| 22491 | 0 |
| 25704 | 0 |
| 28917 | 0 |
| 32130 | 0 |
| 32133 | 0 |

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
| embedding model and vector dimensions | 0.745 | 5 | **Q:** "Find VGS connector implementation files in the hyperswitch repository" *… |
| steward quality scoring and pruning | 0.757 | 5 | 4. Quality feedback ​ When knowledge items are retrieved, memoryd records that… |

**Average top score across all probes:** 0.761

## 8. Rejection Store

- **Total rejections buffered:** 0 / 500 capacity

The rejection store feeds the adaptive noise scorer. After 0 entries,
the scorer has a richer model of what "noise" looks like in real conversations.

## 9. Errors

**Total errors:** 32133 / 32133 (100.0%)

| Error | Count |
|-------|-------|
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 2829; I… | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 4545; I… | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 6183; I… | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 6211; I… | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 11371; … | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 14055; … | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: API error 502: \u003chtml\u003e\r\n\u003chead\… | 40 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 2945; I… | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 11047; … | 1 |
| HTTP 500: {
  "error": "synthesis error: synthesizer: read response: stream error: stream ID 17429; … | 1 |

## 10. Summary

| Metric | Value |
|--------|-------|
| Rows processed | 32133 |
| Store rate | 0.0% |
| Filter rate | 0.0% |
| Explanation leak rate | 0.0% |
| Substantive false-positive rate | 100.0% |
| Memories created | +0 |
| Avg search probe score | 0.761 |
| Median latency | 0 ms |


**Cleanup:** Benchmark memories retained (source=`benchmark-1774678358353901000`). Delete manually if desired.
