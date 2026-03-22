---
sidebar_position: 3
title: The Quality Loop
---

# The Quality Loop

memoryd doesn't just accumulate memories — it maintains them. The **steward** is a background process that scores, prunes, and deduplicates the memory store on a recurring schedule.

## Why quality matters

Without curation, a memory store degrades into noise. Outdated patterns, one-off debugging conversations, and redundant explanations crowd out the signal. The steward solves this by treating memory quality as a feedback loop: memories that help agents answer questions survive; memories that don't, decay and eventually disappear.

## Learning period

For the first **50 retrieval events**, the steward stays hands-off. All memories are kept regardless of quality. This learning period lets the system accumulate enough usage signal before making judgment calls about what to keep.

The quality tracker watches retrieval events and exposes `IsLearning()` — after the threshold is crossed, quality-aware filtering activates across the system.

## The steward sweep

Every **60 minutes** (configurable), the steward runs a three-phase sweep:

### Phase 1: Score

Every memory gets a quality score based on two factors:

**Usage signal:**
$$
\text{baseScore} = \frac{\log_2(\text{hitCount} + 1)}{\log_2(\text{maxHits} + 1)}
$$

Memories that have never been retrieved get a baseline score of `0.5` — neutral, not penalized.

**Time decay:**
$$
\text{decayFactor} = 0.5^{\text{timeSinceLastActive} / \text{halfLife}}
$$

The half-life defaults to **7 days**. A memory retrieved yesterday keeps most of its score. A memory last retrieved a month ago has decayed significantly.

**Final score:**
$$
\text{qualityScore} = \text{baseScore} \times \text{decayFactor} \quad \in [0, 1]
$$

"Last active" is the later of `lastRetrieved` and `createdAt`, so new memories aren't immediately penalized.

### Phase 2: Prune

A memory is deleted when **all three conditions** are true:

1. **Old enough** — created more than 24 hours ago (grace period)
2. **Low quality** — score below `0.1`
3. **Never retrieved** — hit count is exactly 0

All three conditions must be met. A low-quality memory with even a single retrieval survives. A zero-hit memory that was created an hour ago survives. The system is conservative — it only removes memories with genuinely zero evidence of value.

### Phase 3: Merge

Near-duplicate memories waste storage and dilute retrieval results. The steward scans for pairs with cosine similarity ≥ **0.88**:

- **Keep** the memory with the higher hit count (or the older one, if tied)
- **Delete** the other

This handles the common case where the same concept gets stored multiple times across different conversations — slight variations in wording that are semantically identical.

## Tuning

All steward parameters are configurable in `config.yaml`:

```yaml
steward:
  interval_minutes: 60       # sweep frequency
  prune_threshold: 0.1       # quality floor for pruning  
  grace_period_hours: 24     # minimum age before pruning
  decay_half_days: 7         # score half-life in days
  merge_threshold: 0.88      # cosine sim for merge  
  batch_size: 500            # memories per sweep batch
```

Lower `prune_threshold` keeps more borderline memories. Longer `decay_half_days` means memories retain score longer between retrievals. Higher `merge_threshold` means only near-exact duplicates get merged.

See [Configuration](../configuration) for the full reference.
