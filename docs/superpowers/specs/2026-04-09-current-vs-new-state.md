# memex v2 — Current State vs New State
**Date:** 2026-04-09
**Reference spec:** `2026-04-09-memex-v2-design.md`

This document is a complete, file-by-file, function-by-function mapping of what exists today and what it becomes after v2. Nothing is omitted.

---

## Table of Contents

1. [Repository Structure](#1-repository-structure)
2. [Go Module](#2-go-module)
3. [CLI Entry Point](#3-cli-entry-point-cmdmemexmaingo)
4. [Config](#4-config-internalconfiggo)
5. [Data Models](#5-data-models-internalmodelsgo)
6. [Store Interface](#6-store-interface-internalstorgo)
7. [Qdrant Store](#7-qdrant-store-internalqdrantgo)
8. [HTTP Handlers](#8-http-handlers-internalhandlersgo)
9. [HTTP Server & Routes](#9-http-server--routes-internalservergo)
10. [MCP Server](#10-mcp-server-internalmcpgo)
11. [Hook](#11-hook-internalhookgo)
12. [Transcript Parser](#12-transcript-parser-internaltranscriptgo)
13. [Trace Store](#13-trace-store-internaltracergo)
14. [Trace Models](#14-trace-models-internaltracermodelsgo)
15. [Trace Handlers](#15-trace-handlers-internaltracerhandlersgo)
16. [Distill](#16-distill-internaldistillgo)
17. [New Files](#17-new-files)
18. [Docker Compose](#18-docker-compose)
19. [UI — api.ts](#19-ui--apits)
20. [UI — App.tsx](#20-ui--apptsx)
21. [UI — Components](#21-ui--components)
22. [Vite Config](#22-vite-config)
23. [Host Filesystem](#23-host-filesystem)
24. [API Surface — Complete Before/After](#24-api-surface--complete-beforeafter)
25. [MCP Tools — Complete Before/After](#25-mcp-tools--complete-beforeafter)
26. [Hook Events — Before/After](#26-hook-events--beforeafter)

---

## 1. Repository Structure

### Current
```
memex/
├── cmd/memex/main.go
├── internal/
│   ├── config.go
│   ├── distill.go
│   ├── distill_test.go
│   ├── handlers.go
│   ├── handlers_test.go
│   ├── hook.go
│   ├── mcp.go
│   ├── models.go
│   ├── models_test.go
│   ├── qdrant.go
│   ├── qdrant_test.go
│   ├── server.go
│   ├── store.go
│   ├── tracer.go
│   ├── tracer_handlers.go
│   ├── tracer_handlers_test.go
│   ├── tracer_models.go
│   ├── tracer_test.go
│   ├── transcript.go
│   └── transcript_test.go
├── ui/
│   ├── src/
│   │   ├── App.tsx
│   │   ├── App.css
│   │   ├── api.ts
│   │   ├── index.css
│   │   ├── main.tsx
│   │   └── components/
│   │       ├── CheckpointBanner.tsx
│   │       ├── EventRow.tsx
│   │       ├── MemoryList.tsx
│   │       ├── ProjectList.tsx
│   │       ├── SessionList.tsx
│   │       └── TraceTimeline.tsx
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   ├── tsconfig.app.json
│   ├── tsconfig.node.json
│   └── vite.config.ts
├── docker-compose.yml
├── go.mod
└── go.sum
```

### New (additions in bold)
```
memex/
├── cmd/memex/main.go                        ← updated: adds `mine` subcommand
├── internal/
│   ├── classifier.go                        ← NEW: 9-type regex classifier
│   ├── classifier_test.go                   ← NEW
│   ├── config.go                            ← updated: IdentityPath, KGPath fields
│   ├── distill.go                           ← unchanged
│   ├── distill_test.go                      ← unchanged
│   ├── handlers.go                          ← updated: new params, new endpoints
│   ├── handlers_test.go                     ← updated: new test cases
│   ├── hook.go                              ← updated: 3-layer session-start, async mining
│   ├── kg.go                                ← NEW: KnowledgeGraph (SQLite WAL)
│   ├── kg_test.go                           ← NEW
│   ├── kg_handlers.go                       ← NEW: HTTP handlers for /facts/*
│   ├── mcp.go                               ← updated: 13 tools replacing 4
│   ├── miner.go                             ← NEW: transcript miner
│   ├── miner_test.go                        ← NEW
│   ├── models.go                            ← updated: topic + memory_type fields
│   ├── models_test.go                       ← updated
│   ├── qdrant.go                            ← updated: new indexes, filter support
│   ├── qdrant_test.go                       ← updated
│   ├── server.go                            ← updated: new routes, KG init
│   ├── store.go                             ← updated: interface with new filter params
│   ├── tracer.go                            ← unchanged
│   ├── tracer_handlers.go                   ← unchanged
│   ├── tracer_handlers_test.go              ← unchanged
│   ├── tracer_models.go                     ← unchanged
│   ├── tracer_test.go                       ← unchanged
│   ├── transcript.go                        ← updated: returns ConversationTurn not []string
│   └── transcript_test.go                   ← updated
├── ui/
│   ├── src/
│   │   ├── App.tsx                          ← unchanged (v2 — UI redesign is sub-project 6)
│   │   ├── App.css                          ← unchanged
│   │   ├── api.ts                           ← updated: new API calls for facts, pinned, mining
│   │   ├── index.css                        ← unchanged
│   │   ├── main.tsx                         ← unchanged
│   │   └── components/                      ← unchanged (UI redesign is separate)
│   └── ...
├── docs/
│   └── superpowers/
│       └── specs/
│           ├── 2026-04-09-memex-v2-design.md
│           └── 2026-04-09-current-vs-new-state.md
├── docker-compose.yml                       ← updated: host volume mount
├── go.mod                                   ← updated: modernc.org/sqlite added
└── go.sum                                   ← updated
```

---

## 2. Go Module

### Current (`go.mod`)
```go
module github.com/shivamvarshney/memex

go 1.26.1

require (
    github.com/google/uuid v1.6.0
    github.com/mark3labs/mcp-go v0.20.0
    github.com/yosida95/uritemplate/v3 v3.0.2
)
```

### New
```go
module github.com/shivamvarshney/memex

go 1.26.1

require (
    github.com/google/uuid v1.6.0
    github.com/mark3labs/mcp-go v0.20.0
    github.com/yosida95/uritemplate/v3 v3.0.2
    modernc.org/sqlite v1.x.x              // NEW: pure Go SQLite, no CGO
)
```

**Why `modernc.org/sqlite`:** Pure Go implementation — no CGO, no gcc in Docker build stage. Slightly slower than `mattn/go-sqlite3` at high concurrency but negligible at our scale (hundreds of triples, single writer).

---

## 3. CLI Entry Point (`cmd/memex/main.go`)

### Current
```
Commands:
  memex serve                          — start HTTP server
  memex mcp                            — start MCP stdio server
  memex hook <event>                   — run a Claude Code hook handler
    events: session-start, session-stop, pre-tool-use, post-tool-use
```

### New
```
Commands:
  memex serve                          — start HTTP server (unchanged)
  memex mcp                            — start MCP stdio server (unchanged)
  memex hook <event>                   — run hook handler (unchanged)
  memex mine <path>                    — NEW: mine transcripts from path into memories
    path: file or directory of Claude Code JSONL transcripts
    connects to localhost:8765 and POSTs mined memories
    example: memex mine ~/.claude/projects/
```

**Change:** Add `case "mine":` branch calling `memex.RunMine(os.Args[2])`.

---

## 4. Config (`internal/config.go`)

### Current
```go
type Config struct {
    Port      string    // env: PORT, default "8765"
    QdrantURL string    // env: QDRANT_URL, default "http://localhost:6333"
    OllamaURL string    // env: OLLAMA_URL, default "http://localhost:11434"
}

const defaultMemexURL = "http://localhost:8765"

func getMemexURL() string   // reads MEMEX_URL env, fallback to defaultMemexURL
func LoadConfig() Config
```

### New
```go
type Config struct {
    Port         string    // env: PORT, default "8765"
    QdrantURL    string    // env: QDRANT_URL, default "http://localhost:6333"
    OllamaURL    string    // env: OLLAMA_URL, default "http://localhost:11434"
    IdentityPath string    // NEW: env: MEMEX_IDENTITY_PATH, default "/root/.memex/identity.md"
    KGPath       string    // NEW: env: MEMEX_KG_PATH, default "/root/.memex/knowledge_graph.db"
}

const defaultMemexURL = "http://localhost:8765"

func getMemexURL() string   // unchanged
func LoadConfig() Config    // updated: reads two new env vars
```

**Note:** In Docker the paths resolve to `/root/.memex/` (inside container), which maps to `~/.memex/` on host via the volume mount. The hook binary runs on the host so for hook usage `IdentityPath` defaults to `~/.memex/identity.md` via `os.ExpandEnv`.

---

## 5. Data Models (`internal/models.go`)

### Current
```go
type Memory struct {
    ID           string    `json:"id"`
    Text         string    `json:"text"`
    Project      string    `json:"project"`
    Source       string    `json:"source"`
    Timestamp    time.Time `json:"timestamp"`
    Importance   float32   `json:"importance"`
    Tags         []string  `json:"tags"`
    LastAccessed time.Time `json:"last_accessed"`
}

type SaveMemoryRequest struct {
    Text       string   `json:"text"`
    Project    string   `json:"project"`
    Source     string   `json:"source"`
    Importance float32  `json:"importance"`
    Tags       []string `json:"tags"`
}

type SearchResponse struct {
    Memories []Memory `json:"memories"`
}
```

### New
```go
// Valid memory types — enforced at handler layer
var ValidMemoryTypes = map[string]bool{
    "decision": true, "preference": true, "event": true,
    "discovery": true, "advice": true, "problem": true,
    "context": true, "procedure": true, "rationale": true,
}

type Memory struct {
    ID           string    `json:"id"`
    Text         string    `json:"text"`
    Project      string    `json:"project"`
    Topic        string    `json:"topic"`        // NEW: slug e.g. "auth-migration"
    MemoryType   string    `json:"memory_type"`  // NEW: one of 9 ValidMemoryTypes
    Source       string    `json:"source"`
    Timestamp    time.Time `json:"timestamp"`
    Importance   float32   `json:"importance"`
    Tags         []string  `json:"tags"`
    LastAccessed time.Time `json:"last_accessed"`
}

type SaveMemoryRequest struct {
    Text       string   `json:"text"`
    Project    string   `json:"project"`
    Topic      string   `json:"topic"`        // NEW: optional, defaults to project name
    MemoryType string   `json:"memory_type"`  // NEW: required, validated against ValidMemoryTypes
    Source     string   `json:"source"`
    Importance float32  `json:"importance"`
    Tags       []string `json:"tags"`
}

type SearchResponse struct {
    Memories []Memory `json:"memories"`
}

// NEW: KG models
type Fact struct {
    ID         string `json:"id"`
    Subject    string `json:"subject"`
    Predicate  string `json:"predicate"`
    Object     string `json:"object"`
    ValidFrom  string `json:"valid_from,omitempty"`
    ValidUntil string `json:"valid_until,omitempty"`
    Source     string `json:"source,omitempty"`
    CreatedAt  string `json:"created_at"`
}

type RecordFactRequest struct {
    Subject   string `json:"subject"`
    Predicate string `json:"predicate"`
    Object    string `json:"object"`
    ValidFrom string `json:"valid_from,omitempty"`
    Source    string `json:"source,omitempty"`
    Singular  bool   `json:"singular"`  // if true, close existing active fact for same subject+predicate
}

type KGStats struct {
    TotalFacts    int            `json:"total_facts"`
    ActiveFacts   int            `json:"active_facts"`
    ExpiredFacts  int            `json:"expired_facts"`
    EntityCount   int            `json:"entity_count"`
    PredicateTypes map[string]int `json:"predicate_types"`
}

// NEW: Mining models
type ConversationTurn struct {
    Role string // "user" or "assistant"
    Text string
}

type MineRequest struct {
    Path    string `json:"path"`
    Project string `json:"project"`
}

type MineResponse struct {
    Status  string `json:"status"`
    Path    string `json:"path"`
}
```

---

## 6. Store Interface (`internal/store.go`)

### Current
```go
type Store interface {
    Init(ctx context.Context) error
    SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)
    SearchMemories(ctx context.Context, query, project string, limit int) ([]Memory, error)
    ListMemories(ctx context.Context, project string, limit int) ([]Memory, error)
    DeleteMemory(ctx context.Context, id string) error
    Health(ctx context.Context) error
}
```

### New
```go
type Store interface {
    Init(ctx context.Context) error
    SaveMemory(ctx context.Context, req SaveMemoryRequest) (Memory, error)

    // SearchMemories: memoryType and topic are optional filters ("" = no filter)
    SearchMemories(ctx context.Context, query, project, memoryType, topic string, limit int) ([]Memory, error)

    // ListMemories: memoryType and topic are optional filters ("" = no filter)
    ListMemories(ctx context.Context, project, memoryType, topic string, limit int) ([]Memory, error)

    // NEW: returns memories with importance >= 0.9 for the project, sorted by importance desc
    PinnedMemories(ctx context.Context, project string) ([]Memory, error)

    // NEW: update importance of a single memory to 1.0
    PinMemory(ctx context.Context, id string) error

    // NEW: find similar memories by embedding the candidate text
    FindSimilar(ctx context.Context, text, project string, limit int) ([]Memory, error)

    DeleteMemory(ctx context.Context, id string) error
    Health(ctx context.Context) error
}
```

**Breaking changes:**
- `SearchMemories` signature gains `memoryType, topic string` params
- `ListMemories` signature gains `memoryType, topic string` params

All callers (handlers, MCP, hook) updated accordingly.

---

## 7. Qdrant Store (`internal/qdrant.go`)

### Current — methods and behaviour

| Method | Current behaviour |
|---|---|
| `Init` | Checks vector size, migrates if not 768, calls `createCollection` |
| `createCollection` | Creates `memories` collection (768-dim cosine) + `text` full-text index |
| `SaveMemory` | Embeds text via Ollama, upserts point with payload: `text, project, source, timestamp, importance, tags, last_accessed` |
| `SearchMemories(query, project, limit)` | Embeds query, Qdrant vector search, optional `project` filter |
| `ListMemories(project, limit)` | Scroll all memories, score by 60% importance + 40% recency, sort, trim |
| `DeleteMemory(id)` | Delete point by ID |
| `Health` | GET /healthz |
| `embed` | POST to Ollama `/api/embeddings` with `nomic-embed-text` |
| `scroll` | POST to Qdrant `/points/scroll` |
| `put` | Generic PUT to Qdrant |

### New — changes per method

**`Init`:**
- Drop `memories` collection unconditionally (clean schema migration, minimal data)
- Call `createCollection` fresh — no migration logic needed
- Existing migration code removed

**`createCollection`:**
```
Before:  text (full-text) + project (keyword) indexes only
After:   text (full-text) + project (keyword) + memory_type (keyword) + topic (keyword) indexes
```

**`SaveMemory`:**
```
Before payload: {text, project, source, timestamp, importance, tags, last_accessed}
After payload:  {text, project, topic, memory_type, source, timestamp, importance, tags, last_accessed}
```
- `topic` defaults to `project` if empty
- `memory_type` stored as-is (validated upstream at handler layer)

**`SearchMemories(query, project, memoryType, topic, limit)`:**
```
Before filter: project match only (if project != "")
After filter:  project + optional memory_type + optional topic (all as Qdrant "must" conditions)
```

**`ListMemories(project, memoryType, topic, limit)`:**
```
Before filter: project only
After filter:  project + optional memory_type + optional topic
```
- Scoring function unchanged (60% importance + 40% recency)

**`PinnedMemories(project)` — NEW:**
- Qdrant scroll with filter: `project match` + `importance range [0.9, 1.0]`
- No embedding needed — pure payload filter
- Returns sorted by importance descending

**`PinMemory(id)` — NEW:**
- Qdrant `POST /collections/memories/points/payload` to set `importance = 1.0` for point by ID

**`FindSimilar(text, project, limit)` — NEW:**
- Embeds `text` via Ollama
- Qdrant vector search with optional `project` filter
- Returns top matches with scores — used for duplicate detection

**`pointsToMemories` — updated:**
- Parses `topic` and `memory_type` from payload in addition to existing fields

---

## 8. HTTP Handlers (`internal/handlers.go`)

### Current — all handlers

| Handler | Method + Path | Behaviour |
|---|---|---|
| `Health` | `GET /health` | Returns `{status: ok}` if Qdrant healthy |
| `SaveMemory` | `POST /memories` | Decodes body, validates `text`, calls store |
| `SearchMemories` | `GET /memories` | Reads `context`, `project`, `limit` params, searches or lists |
| `DeleteMemory` | `DELETE /memories/:id` | Deletes by ID |
| `Summarize` | `POST /summarize` | Saves memory with `importance=0.9`, tag `session-summary` |

### New — changes per handler

**`SaveMemory` (`POST /memories`):**
```
Before: validates text only. No type checking.
After:  validates text + memory_type (must be in ValidMemoryTypes). 
        Sets topic default to project if empty.
```

**`SearchMemories` (`GET /memories`):**
```
Before: params: context, project, limit
After:  params: context, project, limit, memory_type (optional), topic (optional)
```

**`DeleteMemory`** — unchanged

**`Health`** — unchanged

**`Summarize`** (`POST /summarize`) — updated:
```
Before: saves with tags=["session-summary"]
After:  saves with memory_type="event", topic="session-summary", importance=0.9
        (checkpoint summaries are events)
```

**NEW handler: `PinnedMemories` (`GET /memories/pinned`):**
```
params: project (required)
returns: {memories: [...]} filtered to importance >= 0.9, sorted desc
used by: session-start hook (L1 layer)
```

**NEW handler: `PinMemory` (`PATCH /memories/:id/pin`):**
```
no body required
sets importance = 1.0 on the memory
returns: updated Memory
```

**NEW handler: `FindSimilar` (`GET /memories/similar`):**
```
params: text (required), project (optional), limit (default 5)
returns: {memories: [...]} with similarity scores
used by: MCP find_similar tool
```

**NEW handler: `MineTranscript` (`POST /mine/transcript`):**
```
body: {path, project}
validates path exists and is readable
starts mining goroutine (non-blocking)
returns: {status: "mining started", path: "..."}
```

---

## 9. HTTP Server & Routes (`internal/server.go`)

### Current routes
```
GET  /health
GET  /memories                  → SearchMemories
POST /memories                  → SaveMemory
DELETE /memories/:id            → DeleteMemory
POST /summarize                 → Summarize
POST /trace/event               → TraceEvent
POST /trace/stop                → TraceStop
GET  /trace/sessions            → ListSessions
GET  /trace/session/:id         → GetSession
GET  /trace/projects            → ListProjects
POST /checkpoint                → Checkpoint
GET  /ui/*                      → static files from ui/dist
```

### New routes (additions only — existing routes unchanged)
```
GET  /memories/pinned           → PinnedMemories     (NEW)
PATCH /memories/:id/pin         → PinMemory          (NEW)
GET  /memories/similar          → FindSimilar        (NEW)
POST /mine/transcript           → MineTranscript     (NEW)
POST /facts                     → RecordFact         (NEW)
GET  /facts                     → QueryFacts         (NEW — params: subject, as_of)
DELETE /facts/:id               → ExpireFact         (NEW — sets valid_until, does not delete)
GET  /facts/timeline            → FactTimeline       (NEW — param: entity)
GET  /facts/stats               → FactStats          (NEW)
```

### `RunServe` changes
```go
// Before:
store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
traceStore := NewTraceStore(cfg.QdrantURL)
store.Init(ctx)
traceStore.Init(ctx)

// After:
store := NewQdrantStore(cfg.QdrantURL, cfg.OllamaURL)
traceStore := NewTraceStore(cfg.QdrantURL)
kg, _ := NewKnowledgeGraph(cfg.KGPath)  // NEW
store.Init(ctx)
traceStore.Init(ctx)
kg.Init()                               // NEW: creates SQLite table + indexes + WAL mode

kgh := NewKGHandlers(kg)               // NEW
// register /facts/* routes
```

---

## 10. MCP Server (`internal/mcp.go`)

### Current — 4 tools

| Tool | Parameters | Behaviour |
|---|---|---|
| `save_memory` | `text` (req), `project`, `importance` | POST /memories |
| `search_memory` | `context` (req), `project` | GET /memories?context=... |
| `list_memories` | `project` | GET /memories?project=... |
| `delete_memory` | `id` (req) | DELETE /memories/:id |

### New — 13 tools

**Memory group (6):**

| Tool | Parameters | Behaviour | Change |
|---|---|---|---|
| `save_memory` | `text` (req), `memory_type` (req, enum 9), `project`, `topic`, `importance` | POST /memories | **Updated**: new required `memory_type`, new optional `topic` |
| `search_memory` | `context` (req), `project`, `memory_type`, `topic` | GET /memories?context=...&memory_type=...&topic=... | **Updated**: new filter params |
| `list_memories` | `project`, `memory_type`, `topic` | GET /memories?... | **Updated**: new filter params |
| `delete_memory` | `id` (req) | DELETE /memories/:id | Unchanged |
| `find_similar` | `text` (req), `project`, `limit` | GET /memories/similar?text=...&project=... | **NEW** |
| `memory_overview` | _(none)_ | Taxonomy + KG stats + memex protocol text | **NEW** |

**Knowledge Graph group (5):**

| Tool | Parameters | Behaviour |
|---|---|---|
| `fact_record` | `subject` (req), `predicate` (req), `object` (req), `valid_from`, `source`, `singular` (bool) | POST /facts |
| `fact_query` | `entity` (req), `as_of` | GET /facts?subject=entity&as_of=... |
| `fact_expire` | `id` (req), `ended` | DELETE /facts/:id (sets valid_until, no real delete) |
| `fact_history` | `entity` (req) | GET /facts/timeline?entity=... |
| `fact_stats` | _(none)_ | GET /facts/stats |

**Pinned group (1):**

| Tool | Parameters | Behaviour |
|---|---|---|
| `pin_memory` | `id` (req) | PATCH /memories/:id/pin |

**Mining group (1):**

| Tool | Parameters | Behaviour |
|---|---|---|
| `digest_session` | `path` (req), `project` | POST /mine/transcript |

### `memory_overview` protocol response (injected text)
The tool returns a JSON object containing:
- `taxonomy`: `{project → {topic → {memory_type → count}}}` built from listing all memories
- `total_memories`: int
- `kg_stats`: from `/facts/stats`
- `protocol`: the memex Memory Protocol string (which type to use when, KG vs Qdrant, pinning rules)

This is the self-describing protocol Claude reads to orient itself — equivalent to mempalace's `status` tool.

---

## 11. Hook (`internal/hook.go`)

### Current — `hookSessionStart`
```
1. getProjectName() → git rev-parse or cwd basename
2. GET /health — if down, output offline warning
3. GET /memories?context="project X session context"&project=X&limit=10
4. Output ALL 10 results as flat <memex-memory> bullet list
```

### New — `hookSessionStart` (3-layer)
```
1. getProjectName() → unchanged
2. GET /health — if down, output offline warning (unchanged)

3. L0 — read ~/.memex/identity.md from disk
   - if file missing, skip silently

4. L1 — GET /memories/pinned?project=X
   - returns importance >= 0.9 memories, up to 10
   - no embedding needed

5. L2 — GET /memories?context="project X session context"&project=X&limit=5
   - same semantic search but limit reduced from 10 → 5
   - results sorted: preference + decision types first

6. Build structured <memex-memory> block:
   [identity]   ← L0 content (if exists)
   [pinned]     ← L1 memories with (type) prefix
   [context]    ← L2 memories with (type) prefix
```

### Current — `hookSessionStop`
```
1. readHookInput()
2. if sessionID empty or service down → output empty
3. POST /trace/stop with {session_id, transcript_path}
4. cleanup /tmp/memex-turn-{sessionID}
```

### New — `hookSessionStop`
```
1–4. unchanged (trace stop fires first)

5. NEW: if transcript_path != ""
   project := getProjectName()
   go func() {
       POST /mine/transcript with {path: transcript_path, project: project}
   }()
   // non-blocking — hook returns immediately, mining happens in background
```

### Current — `hookPreToolUse` / `hookPostToolUse`
Unchanged. These record trace events and timing. No memory involvement.

---

## 12. Transcript Parser (`internal/transcript.go`)

### Current
```go
// ParseTranscript reads a Claude Code JSONL file.
// Returns []string — one reasoning string per tool call (text before tool_use block).
// Used only by TraceStop handler to attach reasoning to trace events.
func ParseTranscript(path string) ([]string, error)

// Internal types (unexported):
type transcriptMessage struct { Role string; Content json.RawMessage }
type contentBlock struct { Type, Text, Name string }
```

### New
```go
// ConversationTurn — one full turn in a Claude Code session (exported, used by miner)
type ConversationTurn struct {
    Role string // "user" or "assistant"
    Text string // full text content of the turn
}

// ParseTranscript — unchanged signature, unchanged behaviour
// Still used by TraceStop handler. Not modified.
func ParseTranscript(path string) ([]string, error)

// NEW: ParseConversation reads a Claude Code JSONL file.
// Returns []ConversationTurn — one entry per message turn (user + assistant).
// Extracts all text content blocks. Skips tool_use blocks (they are trace events, not prose).
// Used by the Miner for memory extraction.
func ParseConversation(path string) ([]ConversationTurn, error)
```

**Why two functions:** `ParseTranscript` is used by `TraceStop` for reasoning attachment — its output format (indexed `[]string`) is tied to `TurnIndex` alignment. Changing it would break trace reasoning. `ParseConversation` is a new independent parser for mining purposes.

---

## 13. Trace Store (`internal/tracer.go`)

### Current
```go
type TraceStore struct { baseURL string; client *http.Client }

Methods:
  Init(ctx)                                     — create traces collection in Qdrant
  SaveEvent(ctx, req TraceEventRequest)         — upsert trace event point
  UpsertReasoning(ctx, eventID, sessionID, reasoning) — patch reasoning on existing point
  ListSessions(ctx, project)                    — scroll + group by session_id
  GetSessionEvents(ctx, sessionID)              — scroll filtered by session_id
  ListProjects(ctx)                             — scroll all, deduplicate project field
  eventsToSessions(events)                      — private: groups events → sessions
```

**Unchanged in v2.** The trace store is independent of the memory schema changes.

---

## 14. Trace Models (`internal/tracer_models.go`)

**Unchanged in v2.**

```go
type TraceEvent    — id, session_id, project, turn_index, tool, input, output, reasoning, duration_ms, timestamp, skill
type Session       — session_id, project, start_time, tool_count, skill
type TraceEventRequest — request body for POST /trace/event
type StopRequest   — request body for POST /trace/stop
type CheckpointRequest — request body for POST /checkpoint
```

---

## 15. Trace Handlers (`internal/tracer_handlers.go`)

**Largely unchanged in v2.** One update:

### `Checkpoint` handler — updated
```go
// Before:
mem, err := h.store.SaveMemory(ctx, SaveMemoryRequest{
    Text:       req.Summary,
    Project:    req.Project,
    Source:     "claude-code",
    Importance: 0.9,
    Tags:       []string{"checkpoint", req.Project},
})

// After:
mem, err := h.store.SaveMemory(ctx, SaveMemoryRequest{
    Text:       req.Summary,
    Project:    req.Project,
    Topic:      "checkpoint",          // NEW
    MemoryType: "event",               // NEW: checkpoints are events
    Source:     "claude-code",
    Importance: 0.9,
    Tags:       []string{"checkpoint"},
})
```

All other trace handlers (`TraceEvent`, `TraceStop`, `ListSessions`, `GetSession`, `ListProjects`) unchanged.

---

## 16. Distill (`internal/distill.go`)

**Unchanged in v2.**

```go
// Distill produces caveman-format checkpoint summary from trace events.
// Used by the checkpoint flow.
func Distill(project string, events []TraceEvent) string
```

---

## 17. New Files

### `internal/kg.go` — Knowledge Graph

```go
package memex

import "database/sql"

type Fact struct { ... }           // see models.go above
type KGStats struct { ... }

type KnowledgeGraph struct {
    db   *sql.DB
    path string
}

// NewKnowledgeGraph opens (or creates) the SQLite DB at path.
func NewKnowledgeGraph(path string) (*KnowledgeGraph, error)

// Init creates the facts table, indexes, and sets WAL mode.
// Safe to call multiple times (CREATE TABLE IF NOT EXISTS).
func (kg *KnowledgeGraph) Init() error

// RecordFact inserts a new triple.
// If singular=true, closes any currently active fact for same subject+predicate first.
// Idempotent: if identical active triple exists (same s/p/o, no valid_until), returns existing ID.
func (kg *KnowledgeGraph) RecordFact(subject, predicate, object, validFrom, source string, singular bool) (string, error)

// ExpireFact sets valid_until on the fact matching id.
// Does NOT delete — fact is preserved for historical queries.
func (kg *KnowledgeGraph) ExpireFact(id, ended string) error

// QueryEntity returns all facts about entity (as subject or object).
// If asOf != "", filters to facts valid at that date.
// If asOf == "", returns only currently active facts (valid_until IS NULL).
func (kg *KnowledgeGraph) QueryEntity(entity, asOf string) ([]Fact, error)

// History returns all facts (active + expired) for entity, ordered by valid_from ASC.
func (kg *KnowledgeGraph) History(entity string) ([]Fact, error)

// Stats returns aggregate counts.
func (kg *KnowledgeGraph) Stats() (KGStats, error)
```

**SQLite schema:**
```sql
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS facts (
    id          TEXT PRIMARY KEY,
    subject     TEXT NOT NULL,
    predicate   TEXT NOT NULL,
    object      TEXT NOT NULL,
    valid_from  TEXT,
    valid_until TEXT,
    source      TEXT,
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_facts_subject           ON facts(subject);
CREATE INDEX IF NOT EXISTS idx_facts_object            ON facts(object);
CREATE INDEX IF NOT EXISTS idx_facts_subject_predicate ON facts(subject, predicate);
CREATE INDEX IF NOT EXISTS idx_facts_valid_until       ON facts(valid_until);
```

---

### `internal/kg_handlers.go` — KG HTTP Handlers

```go
type KGHandlers struct { kg *KnowledgeGraph }

func NewKGHandlers(kg *KnowledgeGraph) *KGHandlers

// POST /facts
// body: RecordFactRequest
// returns: Fact (201 Created)
func (h *KGHandlers) RecordFact(w http.ResponseWriter, r *http.Request)

// GET /facts?subject=X&as_of=YYYY-MM-DD
// returns: {facts: []Fact}
func (h *KGHandlers) QueryFacts(w http.ResponseWriter, r *http.Request)

// DELETE /facts/:id?ended=YYYY-MM-DD
// sets valid_until, does NOT delete the row
// returns: 204 No Content
func (h *KGHandlers) ExpireFact(w http.ResponseWriter, r *http.Request)

// GET /facts/timeline?entity=X
// returns: {entity, timeline: []Fact}
func (h *KGHandlers) FactTimeline(w http.ResponseWriter, r *http.Request)

// GET /facts/stats
// returns: KGStats
func (h *KGHandlers) FactStats(w http.ResponseWriter, r *http.Request)
```

---

### `internal/classifier.go` — 9-Type Regex Classifier

```go
package memex

type ClassifyResult struct {
    MemoryType string
    Confidence float64  // 0.0–1.0
}

type Classifier struct {
    markers map[string][]string
}

func NewClassifier() *Classifier

// Classify scores text against all 9 marker sets.
// Returns the highest-scoring type and a confidence value.
// Returns ("", 0) if no type scores above the minimum threshold (0.3).
func (c *Classifier) Classify(text string) ClassifyResult

// internal helpers:
func (c *Classifier) score(text string, markers []string) float64
func (c *Classifier) extractProse(text string) string   // strips code blocks/lines
func (c *Classifier) disambiguate(memType, text string, scores map[string]float64) string
func (c *Classifier) sentiment(text string) string      // "positive", "negative", "neutral"
func (c *Classifier) hasResolution(text string) bool
```

**Marker sets:**
```go
var typeMarkers = map[string][]string{
    "decision":   {"let's use", "we decided", "went with", "rather than", "architecture",
                   "approach", "strategy", "configure", "default", "trade-off", "pattern", "framework"},
    "preference": {"i prefer", "always use", "never use", "don't use", "i like", "i hate",
                   "my rule", "we always", "we never", "snake_case", "camel_case", "please always"},
    "event":      {"session on", "we met", "sprint", "release", "last week", "deployed",
                   "shipped", "launched", "milestone", "standup", "v1.", "v2.", "version"},
    "discovery":  {"it works", "figured out", "turns out", "the trick is", "realized",
                   "breakthrough", "finally", "now i understand", "the key is", "nailed it",
                   "found out", "discovered"},
    "advice":     {"you should", "recommend", "best practice", "the answer is", "suggestion",
                   "consider", "try using", "better to", "i suggest", "worth trying"},
    "problem":    {"bug", "error", "crash", "doesn't work", "root cause", "the fix",
                   "workaround", "broken", "failing", "issue", "not working", "keeps failing"},
    "context":    {"works on", "responsible for", "reports to", "team", "owns",
                   "based in", "member of", "leads", "assigned to", "manages"},
    "procedure":  {"steps to", "how to", "workflow", "always run", "first run",
                   "then run", "pipeline", "process", "to deploy", "to build", "to test"},
    "rationale":  {"the reason we", "chose over", "trade-off", "we rejected", "instead of",
                   "because we need", "pros and cons", "we considered", "the reason is"},
}
```

**Disambiguation rules:**
- `problem` + resolution markers → `discovery`
- `problem` + positive sentiment + no resolution → `discovery`

---

### `internal/miner.go` — Transcript Miner

```go
package memex

type Miner struct {
    classifier *Classifier
    store      Store
}

func NewMiner(classifier *Classifier, store Store) *Miner

// MineTranscript parses a single JSONL transcript file, classifies each turn,
// and saves qualifying memories via store.SaveMemory.
// Returns the list of memories saved and any error.
func (m *Miner) MineTranscript(ctx context.Context, path, project string) ([]Memory, error)

// MineDir walks a directory, finds all .jsonl files, and calls MineTranscript on each.
func (m *Miner) MineDir(ctx context.Context, dirPath, project string) (int, error)

// inferTopic extracts a topic slug from text by finding the most prominent
// noun phrase (simple heuristic: longest repeated 1-2 word sequence, lowercased, hyphenated).
// Falls back to project name if nothing useful found.
func (m *Miner) inferTopic(text string) string

// isDuplicate embeds the text and checks similarity against existing memories for the project.
// Returns true if any existing memory has cosine similarity > 0.92.
func (m *Miner) isDuplicate(ctx context.Context, text, project string) bool
```

**Mining pipeline per turn:**
```
1. ParseConversation(path) → []ConversationTurn
2. For each turn (skip turns with len < 50 chars):
   a. classifier.Classify(turn.Text) → {MemoryType, Confidence}
   b. if Confidence < 0.3 → skip
   c. isDuplicate(turn.Text, project) → if true, skip
   d. inferTopic(turn.Text) → topic slug
   e. store.SaveMemory({Text, Project, Topic, MemoryType, Source="transcript-miner", Importance=confidence})
3. Return saved memories
```

**`RunMine(path string)` — new top-level function called by CLI:**
```go
func RunMine(path string) {
    // walks path (file or dir)
    // POSTs each mined memory to localhost:8765/memories
    // prints summary: "mined N memories from X transcripts"
}
```

---

## 18. Docker Compose

### Current (`docker-compose.yml`)
```yaml
services:
  memex:
    build: .
    ports:
      - "8765:8765"
    environment:
      - QDRANT_URL=http://qdrant:6333
      - OLLAMA_URL=http://host.docker.internal:11434
    depends_on:
      qdrant:
        condition: service_healthy
    restart: unless-stopped

  qdrant:
    image: qdrant/qdrant:v1.13.0
    ports:
      - "6333:6333"
    volumes:
      - qdrant_data:/qdrant/storage
    healthcheck:
      test: ["CMD-SHELL", "bash -c 'cat < /dev/null > /dev/tcp/localhost/6333'"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped

  ui:
    image: node:22-alpine
    working_dir: /app
    volumes:
      - ./ui:/app
      - ui_node_modules:/app/node_modules
    ports:
      - "5173:5173"
    command: sh -c "npm install && npm run dev"
    depends_on:
      - memex
    restart: unless-stopped

volumes:
  qdrant_data:
  ui_node_modules:
```

### New
```yaml
services:
  memex:
    build: .
    ports:
      - "8765:8765"
    environment:
      - QDRANT_URL=http://qdrant:6333
      - OLLAMA_URL=http://host.docker.internal:11434
      - MEMEX_IDENTITY_PATH=/root/.memex/identity.md     # NEW
      - MEMEX_KG_PATH=/root/.memex/knowledge_graph.db    # NEW
    volumes:
      - ~/.memex:/root/.memex                            # NEW: host mount
    depends_on:
      qdrant:
        condition: service_healthy
    restart: unless-stopped

  # qdrant: unchanged
  # ui: unchanged

volumes:
  qdrant_data:
  ui_node_modules:
```

**What `~/.memex/` contains on host:**
```
~/.memex/
  identity.md            ← user creates this manually (L0 identity)
  knowledge_graph.db     ← auto-created by memex serve on first run
```

---

## 19. UI — `api.ts`

### Current exports
```typescript
export interface Session { session_id, project, start_time, tool_count, skill }
export interface Memory  { id, text, project, source, timestamp, importance, tags, last_accessed }
export interface TraceEvent { id, session_id, project, turn_index, tool, input, output, reasoning, duration_ms, timestamp, skill }

export async function fetchProjects(): Promise<string[]>
export async function fetchSessions(project: string): Promise<Session[]>
export async function fetchSessionEvents(sessionId: string): Promise<TraceEvent[]>
export async function fetchMemories(project: string): Promise<Memory[]>
```

### New
```typescript
export interface Session    { ... }     // unchanged
export interface TraceEvent { ... }     // unchanged

// Memory gains two fields
export interface Memory {
  id: string
  text: string
  project: string
  topic: string           // NEW
  memory_type: string     // NEW: one of 9 types
  source: string
  timestamp: string
  importance: number
  tags: string[]
  last_accessed: string
}

// NEW: Knowledge graph types
export interface Fact {
  id: string
  subject: string
  predicate: string
  object: string
  valid_from?: string
  valid_until?: string
  source?: string
  created_at: string
}

export interface KGStats {
  total_facts: number
  active_facts: number
  expired_facts: number
  entity_count: number
  predicate_types: Record<string, number>
}

// Existing fetch functions — unchanged
export async function fetchProjects(): Promise<string[]>
export async function fetchSessions(project: string): Promise<Session[]>
export async function fetchSessionEvents(sessionId: string): Promise<TraceEvent[]>
export async function fetchMemories(project: string): Promise<Memory[]>

// NEW fetch functions
export async function fetchPinnedMemories(project: string): Promise<Memory[]>
export async function fetchFacts(subject: string, asOf?: string): Promise<Fact[]>
export async function fetchFactTimeline(entity: string): Promise<Fact[]>
export async function fetchKGStats(): Promise<KGStats>
export async function fetchMemoryOverview(): Promise<{taxonomy: object, total: number, kg_stats: KGStats}>
```

---

## 20. UI — `App.tsx`

### Current
- Tab-based layout: `traces` | `memories`
- Left sidebar: ProjectList + SessionList (when traces tab active)
- Main area: TraceTimeline or MemoryList

### New
- **Unchanged for v2 backend work** — UI redesign is a separate sub-project (v2 UI)
- However `App.tsx` will be minimally updated to pass `memory_type` and `topic` to `MemoryList` props for filtering (the data is now available)
- Tab structure unchanged

---

## 21. UI — Components

### `MemoryList.tsx`

**Current:** Renders memory cards with `importance`, `tags`, `text`, `source`, `timestamp`.

**New:** Renders `memory_type` as a coloured badge and `topic` as a secondary label. No structural changes — additive only.

```typescript
// New: memory type badge colours
const TYPE_COLORS: Record<string, string> = {
  decision:   'bg-blue-900 text-blue-300',
  preference: 'bg-purple-900 text-purple-300',
  event:      'bg-zinc-700 text-zinc-300',
  discovery:  'bg-green-900 text-green-300',
  advice:     'bg-yellow-900 text-yellow-300',
  problem:    'bg-red-900 text-red-300',
  context:    'bg-indigo-900 text-indigo-300',
  procedure:  'bg-orange-900 text-orange-300',
  rationale:  'bg-teal-900 text-teal-300',
}
```

### `ProjectList.tsx` — unchanged
### `SessionList.tsx` — unchanged
### `TraceTimeline.tsx` — unchanged
### `EventRow.tsx` — unchanged
### `CheckpointBanner.tsx` — unchanged

---

## 22. Vite Config

### Current proxy routes
```typescript
proxy: {
  '/trace': 'http://memex:8765',
  '/checkpoint': 'http://memex:8765',
  '/memories': 'http://memex:8765',
}
```

### New proxy routes
```typescript
proxy: {
  '/trace': 'http://memex:8765',
  '/checkpoint': 'http://memex:8765',
  '/memories': 'http://memex:8765',
  '/facts': 'http://memex:8765',      // NEW: KG endpoints
  '/mine': 'http://memex:8765',       // NEW: mining endpoint
}
```

---

## 23. Host Filesystem

### Current (`~/.memex/` does not exist)
```
No ~/.memex directory.
Claude Code settings reference memex binary at ~/go/bin/memex.
All persistence is inside Docker volumes.
```

### New
```
~/.memex/
  identity.md            ← user creates manually. L0 identity loaded every session.
                            If missing, L0 silently skipped.
                            Example content:
                              "I am Shivam. I build developer tools.
                               Primary project: memex.
                               Stack: Go, Qdrant, React/Vite, Docker."
  knowledge_graph.db     ← SQLite WAL database. Auto-created by `memex serve` on startup.
                            Contains: facts table + 4 indexes.
                            Inspectable with any SQLite browser (TablePlus, DB Browser for SQLite).
```

**`mkdir ~/.memex` is not required** — `memex serve` creates the directory on startup if it doesn't exist (same pattern as mempalace's `config.init()`).

---

## 24. API Surface — Complete Before/After

### Before (11 routes)
```
GET  /health
GET  /memories                  params: context, project, limit
POST /memories                  body: {text, project, source, importance, tags}
DELETE /memories/:id
POST /summarize                 body: {text, project, source}
POST /trace/event               body: TraceEventRequest
POST /trace/stop                body: StopRequest
GET  /trace/sessions            params: project
GET  /trace/session/:id
GET  /trace/projects
POST /checkpoint                body: CheckpointRequest
```

### After (20 routes)
```
GET  /health                                          (unchanged)
GET  /memories                  + memory_type, topic  (updated params)
POST /memories                  + memory_type, topic  (updated body)
DELETE /memories/:id                                  (unchanged)
GET  /memories/pinned           params: project       (NEW)
PATCH /memories/:id/pin                               (NEW)
GET  /memories/similar          params: text, project, limit  (NEW)
POST /summarize                                       (unchanged — now saves as type=event)
POST /trace/event                                     (unchanged)
POST /trace/stop                                      (unchanged)
GET  /trace/sessions                                  (unchanged)
GET  /trace/session/:id                               (unchanged)
GET  /trace/projects                                  (unchanged)
POST /checkpoint                                      (unchanged)
POST /mine/transcript           body: {path, project} (NEW)
POST /facts                     body: RecordFactRequest  (NEW)
GET  /facts                     params: subject, as_of   (NEW)
DELETE /facts/:id               param: ended             (NEW — expires, not hard delete)
GET  /facts/timeline            params: entity           (NEW)
GET  /facts/stats                                        (NEW)
```

---

## 25. MCP Tools — Complete Before/After

### Before (4 tools)
```
save_memory     params: text*, project, importance
search_memory   params: context*, project
list_memories   params: project
delete_memory   params: id*
```
(* = required)

### After (13 tools)
```
// Memory group
save_memory     params: text*, memory_type*, project, topic, importance   (updated)
search_memory   params: context*, project, memory_type, topic              (updated)
list_memories   params: project, memory_type, topic                        (updated)
delete_memory   params: id*                                                (unchanged)
find_similar    params: text*, project, limit                              (NEW)
memory_overview params: none                                               (NEW)

// Knowledge Graph group
fact_record     params: subject*, predicate*, object*, valid_from, source, singular  (NEW)
fact_query      params: entity*, as_of                                               (NEW)
fact_expire     params: id*, ended                                                   (NEW)
fact_history    params: entity*                                                      (NEW)
fact_stats      params: none                                                         (NEW)

// Pinned group
pin_memory      params: id*                                                (NEW)

// Mining group
digest_session  params: path*, project                                     (NEW)
```

---

## 26. Hook Events — Before/After

### `session-start`

| | Before | After |
|---|---|---|
| Identity (L0) | None | Reads `~/.memex/identity.md` from disk |
| Pinned (L1) | None | `GET /memories/pinned?project=X` — importance >= 0.9 |
| Semantic (L2) | `GET /memories?context=...&limit=10` (flat, untyped) | `GET /memories?context=...&limit=5` (type-prioritized: preference+decision first) |
| Output format | `<memex-memory>\n- bullet\n- bullet\n</memex-memory>` | `<memex-memory>\n[identity]\n...\n[pinned]\n- (type) ...\n[context]\n- (type) ...\n</memex-memory>` |
| Token budget | ~10 × avg memory length (unpredictable) | ~20 (L0) + ~80 (L1, max 10) + ~50 (L2, max 5) ≈ controlled |

### `session-stop`

| | Before | After |
|---|---|---|
| Trace stop | `POST /trace/stop` | unchanged |
| Transcript mining | Nothing | `go POST /mine/transcript` (async, non-blocking) |
| Cleanup | Remove `/tmp/memex-turn-{sessionID}` | unchanged |

### `pre-tool-use` — unchanged
### `post-tool-use` — unchanged
