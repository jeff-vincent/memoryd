---
slug: /
sidebar_position: 1
title: What is memoryd?
---

# memoryd

**Persistent memory for coding agents — institutional knowledge that builds itself.**

Every coding agent session starts cold. The agent has no memory of decisions made yesterday, dead ends explored last week, or conventions your team agreed on a month ago. You compensate by re-explaining context, which is tedious and incomplete.

memoryd fixes this. It's a local daemon that sits between your coding agents and the LLM provider, transparently building a persistent knowledge store from every interaction. Over time, it becomes an institutional memory — of architectural decisions, debugging insights, team patterns, and project-specific context — that every agent session can draw from automatically.

```
Developer → Coding Agent → memoryd → LLM Provider
                              ↕
                     MongoDB Atlas (shared)
```

## Two ways to connect

memoryd exposes two interfaces. Use one or both:

| Interface | How it works | Who populates the store |
|-----------|-------------|------------------------|
| **Proxy mode** | Transparent HTTP proxy — agents don't know it's there. Set `ANTHROPIC_BASE_URL=http://127.0.0.1:7432` and work normally. | Automatic. Every response is chunked, embedded, and stored. |
| **MCP server** | [Model Context Protocol](https://modelcontextprotocol.io/) over stdio. Any MCP-compatible agent can search, store, and manage memories via tool calls. | The agent decides — store explicitly, or just search. |

The proxy populates the store automatically from Claude Code sessions. The MCP server makes that store accessible to **any** agentic tool — Claude Code, Cursor, Windsurf, Cline, custom agents, whatever speaks MCP. An agent doesn't need to write anything to benefit from reading what's already there.

## Why this matters for teams

When your team connects to a **shared MongoDB Atlas cluster**, every developer's agent sessions feed the same knowledge store. One person debugs a gnarly infrastructure issue — next week, another developer's agent already knows the resolution. Architectural decisions, API patterns, deployment gotchas — they accumulate organically just from people doing their work.

No one writes documentation. No one maintains a wiki. The knowledge web builds itself.

→ [Team Knowledge Hub](team-knowledge-hub) explores this in depth.

## What happens under the hood

The system is simple on the surface but precise underneath:

1. **[Read path](how-it-works/read-path)** — Every prompt is embedded and searched against the store. Relevant memories are injected into the system prompt. The agent sees prior context without anyone asking for it.

2. **[Write path](how-it-works/write-path)** — Every response is chunked at paragraph boundaries, scrubbed of secrets, batch-embedded, deduplicated, and stored asynchronously. Zero latency added.

3. **[Quality loop](how-it-works/quality-loop)** — A background steward scores memories by usage and recency, prunes noise, and merges near-duplicates. The store self-maintains.

4. **[Hybrid search](how-it-works/hybrid-search)** — On Atlas proper, retrieval combines vector similarity, full-text keyword matching, and diversity re-ranking. On local dev, plain vector search.

## Quick start

```bash
brew install memoryd         # or: go install github.com/kindling-sh/memoryd/cmd/memoryd@latest
memoryd start                # connects to MongoDB, starts embedding model
export ANTHROPIC_BASE_URL=http://127.0.0.1:7432
```

That's it. Launch your coding agent and work normally. Memory builds in the background.

→ [Getting Started](getting-started) has the full setup guide.
