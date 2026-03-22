---
sidebar_position: 5
title: Team Knowledge Hub
---

# Team Knowledge Hub

memoryd isn't just a tool for individual developers. It's a knowledge layer for teams.

## The problem

Every team has institutional knowledge — how the deploy pipeline works, why that config flag exists, what the payment service expects in edge cases. This knowledge lives in people's heads, scattered Slack threads, outdated wiki pages, and tribal memory that walks out the door when someone leaves.

No one writes documentation. No one maintains a wiki. The knowledge that actually matters — the operational, contextual, hard-won understanding of how things work — stays invisible.

## The vision

Point every team member's coding agent at a **shared MongoDB Atlas cluster**. Each person works normally. Their memoryd instance captures what they learn, what they ask about, what they debug.

The steward curates. Duplicates merge. Stale memories decay. High-signal memories rise to the top. Over time, the team builds a living knowledge web — not through documentation effort, but through the natural act of working.

```
Developer A (Claude Code)  ──→ shared Atlas cluster ←── Developer B (Cursor + MCP)
                                      ↕
Developer C (MCP read-only) ←── steward curation ──→ Developer D (proxy mode)
```

Every agent session becomes a contribution. Every question becomes a signal about what matters. The knowledge accumulates organically and benefits everyone.

## How it works

### Shared store

All team members configure their memoryd instance to use the same Atlas connection string:

```yaml
# ~/.memoryd/config.yaml
mongodb_atlas_uri: "mongodb+srv://team-cluster.mongodb.net/?retryWrites=true"
atlas_mode: true
```

That's it. The memories, retrievals, and quality signals all flow into the same database. Atlas handles the multi-tenancy at the infrastructure level.

### Different agents, same store

The store is agent-agnostic. Team members can use different tools:

| Team member | Agent | Integration | Contribution |
|---|---|---|---|
| Alice | Claude Code | Proxy mode | Automatic capture of all sessions |
| Bob | Cursor | MCP server | Explicit search and store via tools |
| Carol | Custom pipeline | MCP read-only | Reads team knowledge, doesn't write |
| Dave | Claude Code | Proxy + MCP | Both automatic capture and explicit tools |

All four benefit from the same knowledge pool. Alice's debugging session about the auth service helps Bob when he encounters the same issue next week.

### Steward at scale

The steward becomes more valuable with a shared store:

- **Cross-session dedup** — when three developers independently learn the same thing, the merge phase (cosine similarity ≥ 0.88) consolidates to a single memory
- **Collective signal** — a memory that gets retrieved across multiple team members' sessions earns a higher quality score faster
- **Natural pruning** — one-off debugging artifacts that never get retrieved across the team decay and disappear

### Source ingestion

Teams can prime the knowledge store with existing documentation:

```
memory_search: "how do deployments work?"
→ (source: source:internal-wiki, score: 0.85) Production deployments require...
→ (source: claude-code, score: 0.78) The deploy script checks for pending migrations...
```

Ingested sources and agent-captured knowledge live side by side. The steward treats them equally — both are scored, pruned, and merged based on actual retrieval value.

## Opt-in / opt-out

memoryd respects individual choice:

- **Full participation** — proxy mode or MCP with writes. Agent sessions feed the store and benefit from it.
- **Read-only** — MCP with search only. Benefit from team knowledge without contributing. Useful for sensitive contexts or trial periods.
- **Isolated** — point at a separate database or run local-only. No team sharing.

There's no forced contribution. The value proposition of the shared store is strong enough that most team members opt in voluntarily — because the more people contribute, the more everyone benefits.

## What builds over time

After weeks of normal work across a team:

- **Deployment procedures** — captured from actual deploy sessions, not stale runbooks
- **Architecture decisions** — the "why" behind design choices, from the conversations where they happened
- **Debugging knowledge** — how to diagnose common issues, from the sessions where they were diagnosed
- **Onboarding context** — new team members' agents immediately know what took others weeks to learn
- **Codebase conventions** — naming patterns, testing approaches, error handling strategies — all captured from practice

The knowledge web is always current because it's built from current work. There's no documentation lag, no stale wiki, no "ask Sarah, she knows."
