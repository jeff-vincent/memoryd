---
sidebar_position: 2
title: How Knowledge is Captured
---

# How Knowledge is Captured

Every AI interaction across your team is automatically processed into reusable knowledge — with zero impact on response speed.

## The flow

```
AI response → Break into pieces → Filter noise → Scrub secrets → Deduplicate → Store
```

### 1. Break into meaningful pieces

Raw conversation text is split into knowledge-sized chunks at natural boundaries — paragraph breaks, logical sections. A function explanation stays together. A list of deployment steps stays together. Only oversized blocks get split further.

### 2. Filter noise

Not everything in a conversation is worth keeping. memoryd automatically filters:

- **Too short** — fragments under 20 characters (code snippets, acknowledgments)
- **Not natural language** — binary data, ASCII art, raw output with less than 40% readable text

### 3. Scrub secrets

**Before anything is stored**, memoryd scrubs sensitive content using 13 detection patterns:

| What's detected | Examples |
|---|---|
| Cloud credentials | AWS access keys, AWS secret keys |
| API tokens | GitHub tokens, Slack tokens, Stripe keys |
| Authentication secrets | JWTs, Bearer tokens, private keys, SSH keys |
| Connection strings | Database URIs with embedded passwords |
| Generic secrets | Key-value pairs containing `password`, `secret`, `token`, `api_key` |

Detected values are replaced with safe placeholders (e.g., `[REDACTED:AWS_KEY]`). **Secrets never enter the shared knowledge store.** This is critical for team deployments where multiple people contribute to the same database.

### 4. Deduplicate

Each new piece of knowledge is compared against what's already in the store:

| Similarity | What happens |
|---|---|
| **Very high** (≥ 92%) | Already known — skip |
| **Moderate** (≥ 75%, from a source) | Related to existing reference material — stored with a link back to the original |
| **Low** | Novel knowledge — stored normally |

This is especially valuable for teams: when three engineers independently learn the same thing about a service, it's stored once — not three times.

### 5. Store

Each surviving piece becomes a knowledge item in the shared MongoDB store, available to every team member's AI tools immediately.

## Why async matters

The entire capture pipeline runs in the background. The AI response streams back to the developer in real-time — memoryd processes it after delivery. Team members never experience any slowdown from the knowledge capture process.

## What builds over time

After a team has been using memoryd for a few weeks, the shared store typically contains:

- **Architecture decisions** — why the team chose specific patterns, from the conversations where those decisions were made
- **Debugging playbooks** — how to diagnose common issues, from actual debugging sessions
- **Deployment knowledge** — environment config, migration procedures, rollback steps
- **Codebase conventions** — naming patterns, error handling approaches, testing strategies
- **Integration details** — how services connect, what APIs expect, edge cases discovered in practice

All of it captured organically, with secrets scrubbed and duplicates merged.
