---
sidebar_position: 2
title: Proxy Mode
---

# Proxy Mode

Proxy mode is the zero-effort way to connect Claude Code (or any Anthropic-based tool) to your team's shared knowledge. Set one environment variable and work normally — knowledge capture happens automatically in the background.

## How it works

memoryd runs a local proxy on each team member's machine. When they point their AI tool at it, the proxy:

1. **On the way out** — searches the shared knowledge store and injects relevant context into the prompt
2. **On the way back** — captures the response, processes it through the [knowledge capture pipeline](../how-it-works/write-path), and stores it

The AI tool doesn't know memoryd is there. The developer doesn't change their workflow. Knowledge just accumulates.

```
AI tool → memoryd proxy (local) → Anthropic API
               ↓                       ↓
     inject team context        capture response
               ↓                       ↓
          shared Atlas store (async, background)
```

## Setup

```bash
memoryd start
export ANTHROPIC_BASE_URL=http://127.0.0.1:7432
```

That's it. Launch Claude Code and work normally.

## What gets captured

Both sides of every conversation feed the knowledge store:

| Source | What's captured |
|---|---|
| **Questions** | What the developer asked — signals what topics matter |
| **Responses** | The AI's answers — architecture decisions, debugging steps, explanations |

Everything goes through the same pipeline — [noise filtering, secret scrubbing, deduplication](../how-it-works/write-path). There's no risk of secrets or garbage entering the shared store.

## Real-time streaming

memoryd handles streaming responses (SSE) natively. The response streams to the developer in real-time; memoryd buffers a copy in the background for processing. Developers never experience any slowdown.

## Built-in dashboard

The proxy also serves a dashboard at `http://localhost:7432` where team members can:

- Browse stored knowledge
- Search the knowledge base
- View quality statistics
- Monitor ingested sources

## When to use proxy vs. MCP

| Use proxy when... | Use MCP when... |
|---|---|
| Your team uses Claude Code | Your team uses Cursor, Windsurf, or other tools |
| You want fully automatic capture | You want agents to control what gets stored |
| You want zero workflow changes | You need integration with non-Anthropic tools |

Many teams use both — proxy for Claude Code users, MCP for everyone else. They all feed the same shared store.
