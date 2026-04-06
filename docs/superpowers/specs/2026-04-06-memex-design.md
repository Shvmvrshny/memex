# memex — Local AI Memory System

**Date:** 2026-04-06  
**Status:** Approved

---

## Problem

AI assistants (Claude Code, ChatGPT, etc.) have four core memory failures:
1. Forget everything between sessions — users repeat context constantly
2. Remember wrong things or contradict prior stored state
3. Memory is opaque — users can't see or control what was stored
4. Memory is siloed — Claude remembers something, ChatGPT doesn't know it

## Solution

A local, portable memory system: a Go HTTP service + Qdrant vector DB running in Docker, exposed to Claude Code via a plugin (hooks + MCP tools). Memories are stored as vector embeddings, retrieved semantically at session start, and injected into Claude's context. Everything stays on the user's machine.

---

## Architecture

```
Claude Code Session
      │
      ▼
┌─────────────────┐
│  memex plugin   │  ← Claude Code plugin (hooks + MCP tools)
└────────┬────────┘
         │ HTTP (localhost:8765)
         ▼
┌─────────────────┐
│  Go HTTP API    │  ← memory service (Docker)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│    Qdrant       │  ← vector DB (Docker, localhost:6333)
└─────────────────┘
```

Two Docker containers. One plugin. One `docker compose up -d` to start everything.

---

## Components

### 1. Go HTTP Service (`memex-service`)

Four endpoints:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/memories?context=<text>&limit=5` | Retrieve top-N relevant memories via semantic search |
| POST | `/memories` | Save a new memory |
| POST | `/summarize` | Summarize and store a session |
| GET | `/health` | Liveness check |

### 2. Claude Code Plugin (`memex`)

```
hooks/
  session-start     → fetch relevant memories, inject into additionalContext
  session-stop      → trigger session summarization
skills/
  memex/SKILL.md    → instructs Claude when/how to save memories
mcp/
  tools             → save_memory, search_memory, list_memories
```

### 3. Docker Compose

```yaml
services:
  memex:
    build: .
    ports: ["8765:8765"]
    depends_on: [qdrant]

  qdrant:
    image: qdrant/qdrant
    ports: ["6333:6333"]
    volumes: ["qdrant_data:/qdrant/storage"]
```

---

## Data Flow

### Session Start
1. Hook fires → reads current project path + git repo name
2. Calls `GET /memories?context=<project context>&limit=5`
3. Go service queries Qdrant for semantically closest memories
4. Returns JSON → injected into Claude's `additionalContext`

### During Session (Claude saves a memory)
1. Claude calls MCP tool: `save_memory("user prefers table-driven tests in Go")`
2. Go service embeds the text via Qdrant's built-in fastembed
3. Stored in Qdrant with metadata

### Session End
1. `SessionStop` hook fires
2. Plugin calls `POST /summarize`
3. Go service stores session summary as a high-importance memory

---

## Memory Schema

```json
{
  "text": "user prefers table-driven tests in Go",
  "embedding": [...],
  "project": "memex",
  "source": "claude-code",
  "timestamp": "2026-04-06T10:00:00Z",
  "importance": 0.8,
  "tags": ["testing", "go", "preference"]
}
```

---

## Embeddings

Qdrant's built-in `fastembed` model handles embedding generation natively — no Python, no sentence-transformers, no external API. Pure Go → Qdrant. Everything local.

---

## Error Handling

**Rule: memory failures are silent and non-fatal. Memory is an enhancement, not a dependency.**

| Failure | Behavior |
|---------|----------|
| Service unreachable at session start | Claude starts normally, no memories injected |
| Qdrant write fails | Log to file, session continues |
| Docker not running | Plugin detects `/health` timeout, skips gracefully |

One user-visible warning only if service is unreachable at session start:
```
<memex> memory service offline — starting without memory context
```

---

## v1 Scope

- Claude Code only
- Memories about the user (preferences, working style, expertise, project decisions)
- Local Docker deployment
- Go HTTP service + Qdrant
- Plugin with hooks + 3 MCP tools

**Out of scope for v1:** cross-platform support (ChatGPT etc.), memory decay/forgetting, multi-user, cloud sync.

## v2 Ideas

- `DELETE /memories/stale` — delete memories not accessed in the last N days (default 30)
- Importance decay — lower importance score over time if a memory is never retrieved
- Manual memory management UI — let users view, edit, delete memories
