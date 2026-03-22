---
sidebar_position: 4
title: Hybrid Search
---

# Hybrid Search

When connected to a MongoDB Atlas cluster (the recommended setup for teams), memoryd uses a multi-stage search pipeline that combines two complementary approaches to find the most relevant knowledge.

## Why hybrid?

Consider a developer asking about `ERR_CONN_REFUSED`. A meaning-based search finds knowledge about connection errors in general — useful, but not specific. A keyword-based search finds the exact item that mentions that precise error code. Hybrid search combines both signals to deliver the best of each.

This matters for teams: the shared knowledge store contains a mix of high-level architectural context and specific technical details. Hybrid search surfaces both.

## Two search modes

memoryd automatically selects the best search strategy based on your database:

| Setup | Strategy | Best for |
|---|---|---|
| **Atlas cluster** (`atlas_mode: true`) | Hybrid search | Teams — more accurate, diverse results |
| **Local MongoDB** | Vector search only | Solo development — simpler, still effective |

No configuration change needed beyond `atlas_mode`. The upgrade happens automatically when you connect to Atlas.

## How hybrid search works

The pipeline runs in four stages:

### 1. Meaning-based search (semantic)

The question is converted to a mathematical representation and compared against all stored knowledge by conceptual similarity. This finds items that are *about the same thing*, even if they use completely different words.

Quality filtering is applied at this stage — low-scoring knowledge items are excluded from results.

### 2. Keyword-based search (lexical)

A parallel text search looks for exact keyword matches — acronyms, error codes, class names, specific terms. This catches items that semantic search might rank lower because the wording is different but the exact match is valuable.

### 3. Rank fusion

The two result lists are combined using Reciprocal Rank Fusion (RRF), which merges rankings from different sources without needing to calibrate their scoring scales. An item ranked highly by both searches scores highest; an item found by only one search still appears.

### 4. Diversity optimization

The fused results are re-ranked to maximize diversity. Without this step, the top results might be five slight variations of the same knowledge. The diversity pass (Maximal Marginal Relevance) ensures results cover *different aspects* of the question — giving the AI tool a broader, more useful set of context.

## What this means for teams

For a team of 10+ contributors, the shared knowledge store grows quickly. Hybrid search ensures that:

- **Specific technical details** (error codes, config values, API endpoints) are found even when the question is phrased broadly
- **Conceptual knowledge** (architecture decisions, design rationale) is found even when the question uses different terminology
- **Results are diverse** — the AI tool gets context from multiple angles, not five versions of the same thing
- **Low-quality noise is filtered out** — knowledge that was captured but never proved useful doesn't clutter results
