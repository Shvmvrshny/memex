# Memex Tracer — Design Spec
**Date:** 2026-04-07  
**Status:** Draft

---

## Overview

Extend memex with agent session tracing. Claude Code hooks capture tool events and reasoning in real-time. At session end, a distillation step compresses the session into a caveman-format checkpoint memory. The checkpoint is injected into future sessions automatically — enabling "resume where we left off" across chats.

Tracing is not a separate project. It extends memex because traces exist to create better checkpoints, and checkpoints are memories. Same binary, same port, same Docker instance.

---

## Problem

Claude Code sessions are stateless. Long, complex sessions (reviews, debugging, architecture work) produce no durable record of what the AI did or why. Starting a new chat means starting blind. Compaction summarizes within a session but doesn't carry across sessions, and you have no control over what it keeps.

---

## Solution

Two new capabilities added to memex:

1. **Tracing** — capture every tool call (input, output, reasoning, duration) into a new `traces` Qdrant collection during a session
2. **Checkpointing** — at session end, distill the trace into a compact caveman-format memory that gets injected into the next session automatically

---

## Architecture

```
Claude Code session
  ├── PostToolUse hook → POST /trace/event   → memex server → Qdrant "traces"
  └── Stop hook        → POST /trace/stop
                           ├── read session transcript (~/.claude/projects/<id>/...)
                           ├── enrich trace events with reasoning text
                           ├── save enriched events → Qdrant "traces"
                           └── distill session → POST /memories (caveman summary)

New chat session:
  └── session start → memex injects checkpoint memory → Claude resumes from context
```

One binary (`memex`), one port, one Docker Qdrant instance. New `traces` collection alongside existing `memories` collection.

---

## Data Model

### Trace Event (Qdrant `traces` collection)

Each tool call produces one point:

```json
{
  "id": "uuid",
  "vector": [0.0],
  "payload": {
    "session_id": "uuid",
    "project": "memex",
    "turn_index": 3,
    "type": "tool_call",
    "tool": "Read",
    "input": "internal/qdrant.go",
    "output": "<file contents...>",
    "reasoning": "I need to check how points are saved before adding the new endpoint.",
    "duration_ms": 12,
    "timestamp": "2026-04-07T10:42:03Z",
    "skill": "/review"
  }
}
```

- `reasoning` is empty until `Stop` hook enriches it
- `session_id` groups all events for one Claude Code session
- `skill` is detected from the active gstack skill if present (optional)
- Same dummy vector approach as `memories` — full-text search via Qdrant payload filtering

### Session (derived, not stored separately)

A session is a group of trace events sharing a `session_id`. Session metadata (start time, project, tool count, skill) is derived at query time by aggregating events.

### Checkpoint Memory (stored in `memories` collection)

At `Stop`, memex saves a structured caveman summary as a regular memory:

```
project: memex. session: 2026-04-07 10:42.
done: designed tracer, added traces collection, PostToolUse hook.
decided: extend memex not separate project. hybrid capture.
blocked: session transcript file path needs dynamic detection.
next: Stop hook enrichment, distillation logic, UI.
tools: 14 calls. Read x5, Grep x3, Edit x4, Bash x2.
```

Tags: `["checkpoint", "tracer", "<project>"]`  
Importance: `0.9` (checkpoints are high-priority memories)

---

## New Endpoints

### Trace Ingestion

**`POST /trace/event`**
Called by `PostToolUse` hook after every tool call.

Request:
```json
{
  "session_id": "uuid",
  "project": "memex",
  "turn_index": 3,
  "tool": "Read",
  "input": "internal/qdrant.go",
  "output": "package memex...",
  "duration_ms": 12,
  "timestamp": "2026-04-07T10:42:03Z"
}
```

Response: `{ "id": "uuid" }`

**`POST /trace/stop`**
Called by `Stop` hook when session ends.

Request:
```json
{
  "session_id": "uuid",
  "project": "memex",
  "transcript_path": "/Users/x/.claude/projects/abc/session.jsonl"
}
```

Actions:
1. Read transcript file, extract assistant reasoning per tool call
2. Upsert trace events with reasoning text
3. Distill session → save as checkpoint memory

### Trace Query

**`GET /trace/sessions?project=memex`**  
Returns sessions for a project, sorted by recency. Each session: id, project, start time, tool count, skill.

**`GET /trace/session/:id`**  
Returns all events for a session, sorted by turn_index.

### Project List

**`GET /projects`**  
Returns all distinct project names from both `memories` and `traces` collections.

### Checkpoint

**`POST /checkpoint`**  
Manual trigger — user asks Claude to save a checkpoint mid-session. Claude calls this with a caveman summary it generates from the current conversation.

Request:
```json
{
  "project": "memex",
  "summary": "done: auth. decided: JWT no refresh. next: middleware."
}
```

---

## Hook Configuration

Users add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "",
      "hooks": [{
        "type": "command",
        "command": "curl -s -X POST http://localhost:4002/trace/event -H 'Content-Type: application/json' -d '{\"session_id\":\"$SESSION_ID\",\"project\":\"$PROJECT\",\"tool\":\"$TOOL_NAME\",\"input\":$TOOL_INPUT,\"output\":$TOOL_OUTPUT,\"duration_ms\":$DURATION_MS,\"timestamp\":\"$TIMESTAMP\"}'"
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "curl -s -X POST http://localhost:4002/trace/stop -H 'Content-Type: application/json' -d '{\"session_id\":\"$SESSION_ID\",\"project\":\"$PROJECT\",\"transcript_path\":\"$TRANSCRIPT_PATH\"}'"
      }]
    }]
  }
}
```

Memex tracer runs on port `4002` (memex currently on `4001`). Same binary, different port flag, or same port with new routes — TBD based on implementation.

---

## UI

React + Vite + Tailwind. Built static files served by the Go server at `/ui`.

**Layout:**
```
┌─────────────────┬──────────────────────────────────────────────┐
│  Projects       │  memex — session Apr 7, 10:42 — 14 tool calls │
│  ─────────────  │  ───────────────────────────────────────────── │
│  memex      3   │  Reasoning: "need to check how points are      │
│  tracer     1   │   saved before adding the new endpoint"        │
│  gstack     8   │                                                │
│                 │  10:42:03 ● Read          internal/qdrant.go   │
│                 │           duration: 12ms          ▶ output     │
│  Sessions       │                                                │
│  ─────────────  │  10:42:09 ● Grep          SaveMemory           │
│  ● Apr 7 10:42  │           duration: 8ms           ▶ output     │
│    14 tools     │                                                │
│                 │  10:42:15 ● Edit          internal/qdrant.go   │
│  ○ Apr 6 15:30  │           duration: 24ms          ▶ output     │
│    31 tools     │                                                │
│                 │  ── Checkpoint saved ──────────────────────── │
│                 │  done: designed tracer.                        │
│                 │  decided: extend memex.                        │
│                 │  next: Stop hook, UI.                          │
└─────────────────┴──────────────────────────────────────────────┘
```

- Left: project list → click to filter sessions
- Right: session trace — reasoning at top, tool calls chronological
- Output collapsed by default, expand inline
- Checkpoint summary shown as a visual marker at session end
- Dark theme, monospace font for outputs

---

## Internal Code Structure

```
memex/
  internal/
    tracer.go          # TraceStore — Qdrant ops for traces collection
    tracer_models.go   # TraceEvent, Session structs
    tracer_handlers.go # HTTP handlers for /trace/* and /projects
    distill.go         # Session → caveman summary logic
    transcript.go      # Read + parse ~/.claude session transcript files
  ui/
    src/
      App.tsx
      components/
        ProjectList.tsx
        SessionList.tsx
        TraceTimeline.tsx
        EventRow.tsx
        CheckpointBanner.tsx
    dist/              # built output, served by Go
```

---

## What's Out of Scope (v1)

- Real embeddings / semantic search over reasoning text
- Live session streaming (traces appear after session ends, not in real-time)
- Multi-machine sync
- Trace diffing across sessions
- Token count tracking per tool call

---

## Open Questions

1. **Port:** Same port as memex (4001) with new routes, or separate port (4002)? Leaning same port — one less thing to configure.
2. **Transcript path:** Claude Code's session file path format needs verification — `$TRANSCRIPT_PATH` hook variable may not exist and may need to be derived from `$SESSION_ID`.
3. **Distillation:** Claude does the summarization at Stop time (calls `/checkpoint` itself) or the Go server calls the Anthropic API directly? Claude doing it via hook is simpler for v1.
