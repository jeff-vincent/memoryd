---
sidebar_position: 3
title: Read-Only Mode
---

# Read-Only Mode

Not every agent needs to write. Some should just listen.

## The concept

memoryd's knowledge store can be consumed without contributing to it. An agent connects via MCP, uses `memory_search` to retrieve context, and never calls `memory_store`. The knowledge it reads was built by other agents, other sessions, other team members.

This is the simplest integration pattern and the fastest way to get value from an existing memory store.

## Why read-only?

**Institutional knowledge without the overhead.** A team has been using memoryd for weeks. The store contains hundreds of curated memories about the codebase — architectural decisions, deployment procedures, common pitfalls. A new team member (or a new tool) can immediately benefit from all of that by connecting read-only.

**Agent-agnostic consumption.** The MCP server works with any MCP-compatible client. An agent running through Cursor, Windsurf, or a custom pipeline can read from the same store that Claude Code sessions have been populating. The write side doesn't need to match the read side.

**Opt-out without opting out.** Some environments have policies about what data an AI agent can store. Read-only mode lets those environments still benefit from the knowledge that other (less restricted) environments have built.

**Safe evaluation.** Trying memoryd for the first time? Connect read-only to an existing store. See what the retrieval quality looks like. Decide later whether to enable writes.

## Setup

Configure memoryd as an MCP server:

```json
{
  "mcpServers": {
    "memoryd": {
      "command": "memoryd",
      "args": ["mcp"]
    }
  }
}
```

Then simply don't use any write tools. The agent has access to all 8 MCP tools but only calls `memory_search`. There's no special flag — read-only is a usage pattern, not a configuration.

If you want to enforce read-only at the agent level, configure your agent's system prompt to only use `memory_search`:

```
You have access to a memory_search tool. Use it to look up relevant context
before answering questions about the codebase. Do not store new memories.
```

## What the agent sees

When `memory_search` returns results, the agent gets formatted context:

```
[1] (source: claude-code, score: 0.87)
The payment service validates webhook signatures using HMAC-SHA256...

[2] (source: source:internal-wiki, score: 0.82)
Deployments to production require approval from the #releases channel...
```

Context from proxy sessions, MCP writes, and ingested sources — all surfaced through a single search.

## The bigger picture

Read-only mode is a stepping stone to the [team knowledge hub](../team-knowledge-hub) vision. Every team member's agent sessions populate a shared store. Any team member can connect read-only and benefit from the collective knowledge — even if they use a different agent, a different workflow, or choose not to contribute.
