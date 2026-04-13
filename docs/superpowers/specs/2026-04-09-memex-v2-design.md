# memex v2 — Design Spec
**Date:** 2026-04-09
**Status:** Awaiting implementation

---

## Overview

memex v2 adopts the best ideas from mempalace and improves on them. The goal is a stronger, more structured memory backend for Claude Code — better retrieval, temporal fact tracking, and automatic session mining — while keeping memex's unique advantage: session tracing.

This spec covers 5 sub-projects executed in order. Each depends on the previous.

```
1. Structured Memory     — 9-type taxonomy, topic field, clean schema
2. Memory Layers         — L0 identity, L1 pinned facts, smarter session-start
3. Knowledge Graph       — temporal entity-relationship triples in SQLite (WAL)
4. Transcript Mining     — auto-classify past sessions into typed memories
5. MCP Tools             — 13 tools, self-describing protocol
```

No commits to GitHub until all 5 are complete and user-approved.

---

## Sub-project 1: Structured Memory

### What mempalace does
Stores memories in ChromaDB with `wing` (project), `room` (topic slug), and `hall` (memory type) as metadata fields. 5 hall types: `hall_facts`, `hall_events`, `hall_discoveries`, `hall_preferences`, `hall_advice`. The 2-level hierarchy (wing + room) produces a measured 34% retrieval improvement over flat search. Claude explicitly specifies all three fields on every `add_drawer` call.

### What memex does
Flat schema: `project`, `tags[]`, `importance`, `text`. No memory type. No topic. Tags are informal and unindexed for filtering. Claude has no schema guidance. Retrieval filters only by `project`.

### What mempalace does better
- Structured 2-level hierarchy gives retrieval a head start before semantic search even runs
- Memory type is a first-class indexed field — queries can be scoped to "give me all decisions for this project"
- Claude always knows where it's filing; nothing gets lost in a flat pile

### How we improve
- **9 memory types** (vs mempalace's 5 halls) — adds `problem`, `context`, `procedure`, `rationale` which are specific to coding assistant workflows
- **`topic` field** — free-form slug (e.g. `auth-migration`, `ci-pipeline`). Equivalent to mempalace's room. Combined with `project` gives us the 2-level hierarchy
- **Clean schema migration** — drop and recreate Qdrant collection (minimal existing data). No legacy debt

### Data Model

**`Memory` struct — new fields:**
```go
type Memory struct {
    ID           string    `json:"id"`
    Text         string    `json:"text"`
    Project      string    `json:"project"`
    Topic        string    `json:"topic"`        // NEW: slug e.g. "auth-migration"
    MemoryType   string    `json:"memory_type"`  // NEW: one of 9 types
    Source       string    `json:"source"`
    Timestamp    time.Time `json:"timestamp"`
    Importance   float32   `json:"importance"`
    Tags         []string  `json:"tags"`
    LastAccessed time.Time `json:"last_accessed"`
}
```

**9 memory types:**
| Type | Description | mempalace equivalent |
|---|---|---|
| `decision` | Locked-in choices, architecture resolutions | `hall_facts` |
| `preference` | Coding style, tool choices, habits | `hall_preferences` |
| `event` | Sessions, milestones, deployments | `hall_events` |
| `discovery` | Breakthroughs, insights, "it works" moments | `hall_discoveries` |
| `advice` | Recommendations, solutions, best practices | `hall_advice` |
| `problem` | Bugs, errors, root causes and their fixes | new |
| `context` | Team members, org structure, project relationships | new |
| `procedure` | Build steps, workflows, repeatable processes | new |
| `rationale` | Why a decision was made, trade-offs considered | new |

**`SaveMemoryRequest` — updated:**
```go
type SaveMemoryRequest struct {
    Text       string   `json:"text"`
    Project    string   `json:"project"`
    Topic      string   `json:"topic"`        // NEW: optional, defaults to project
    MemoryType string   `json:"memory_type"`  // NEW: required, one of 9 types
    Source     string   `json:"source"`
    Importance float32  `json:"importance"`
    Tags       []string `json:"tags"`
}
```

### Qdrant Schema Changes

`createCollection` drops and recreates with:
- 768-dim cosine vectors (unchanged)
- Full-text payload index: `text` (unchanged)
- Keyword payload index: `project` (unchanged)
- **NEW** keyword payload index: `memory_type`
- **NEW** keyword payload index: `topic`

`SearchMemories` and `ListMemories` accept optional `memory_type` and `topic` filter params.

### API Changes

`GET /memories` — new optional query params: `memory_type`, `topic`
`POST /memories` — body now includes `memory_type` (required), `topic` (optional)

---

## Sub-project 2: Memory Layers

### What mempalace does
4-layer memory stack: L0 identity (~50 tokens, always loaded), L1 critical facts (~120 tokens AAAK-compressed, always loaded), L2 room recall (on demand), L3 deep search (on demand). Session wake-up loads L0 + L1 only (~170 tokens). Searches fire when needed.

### What memex does
One semantic search on session-start: query = `"project X session context"`, returns top 10 memories, dumps them all into `<memex-memory>`. No prioritization. No pinned facts. No identity layer. Noisy and token-inefficient.

### What mempalace does better
- L0 identity gives the AI a stable self-model every session — who you are, what projects exist, what the AI's role is
- L1 pinned facts guarantee critical preferences/decisions are always present, regardless of semantic search results
- Token budget is controlled — ~170 tokens vs memex's unpredictable dump

### How we improve
- Same 3-layer approach as mempalace but without AAAK compression (we store raw text, same as mempalace's highest-accuracy raw mode)
- L1 is `importance >= 0.9` rather than a separate "critical facts" concept — simpler, same effect
- Type-prioritized L2: `preference` and `decision` types ranked first in semantic results

### Layer Design

**L0 — Identity** (always loaded, ~20 tokens)
- File: `~/.memex/identity.md`
- Plain text written by the user once. Example:
  ```
  I am Shivam. I build developer tools. Primary project: memex.
  Stack: Go, Qdrant, React/Vite, Docker.
  ```
- Read from disk by the hook binary. Never stored in Qdrant.
- If file doesn't exist, L0 is skipped silently.

**L1 — Pinned facts** (always loaded, up to 10 entries)
- All memories where `importance >= 0.9`, filtered by current project
- Retrieved via `GET /memories/pinned?project=X` — no embedding search, pure payload filter
- These are non-negotiables. Critical preferences, locked-in decisions.

**L2 — Semantic context** (5 results, type-prioritized)
- Current semantic search but scoped to project, limited to 5
- Results sorted: `preference` + `decision` types first, then others

**Hook output — structured `<memex-memory>` block:**
```
<memex-memory>
[identity]
I am Shivam. I build developer tools...

[pinned]
- (preference) user prefers table-driven tests in Go
- (decision) using Qdrant + nomic-embed-text for storage

[context]
- (discovery) Ollama must run on host.docker.internal not localhost
</memex-memory>
```

### New API Endpoint
`GET /memories/pinned?project=X` — returns memories with `importance >= 0.9` for project X, sorted by importance descending.

### Docker / File Changes
- `docker-compose.yml`: add `~/.memex:/root/.memex` volume to `memex` service
- `internal/config.go`: add `IdentityPath` config field (default `~/.memex/identity.md`)

---

## Sub-project 3: Knowledge Graph

### What mempalace does
Temporal entity-relationship triples in SQLite (`knowledge_graph.sqlite3` at `~/.mempalace/`). Subject → predicate → object with `valid_from` / `valid_until` validity windows. Invalidation closes the interval rather than deleting. 5 MCP tools: `kg_query`, `kg_add`, `kg_invalidate`, `kg_timeline`, `kg_stats`.

### What memex does
Nothing. No structured fact store. All knowledge is unstructured text in Qdrant. If a fact changes (e.g. a team member moves to a different project), there's no way to record the change or query what was true at a point in time.

### What mempalace does better
- Temporal validity windows — "what was true on date X" is a first-class query
- Entity-centric queries — "what do I know about entity X right now" returns structured facts, not blobs
- Invalidation without deletion — history is preserved, present truth stays clean
- Complements vector search: KG for precision, Qdrant for semantics

### How we improve
- **WAL mode** — `PRAGMA journal_mode=WAL` on every connection. Enables concurrent reads during writes, matching the memex server's architecture (session-start reads + MCP writes happen simultaneously)
- **Singular vs multi-valued predicates** — `fact_record` MCP tool accepts a `singular bool` flag. When `true`, the currently active fact for the same `subject + predicate` is automatically closed (`valid_until = now`) before the new one is inserted. Keeps timelines clean without manual invalidation for common cases
- **Idempotency** — before inserting, check for an identical active triple (same subject + predicate + object + no valid_until). If found, return existing ID. Prevents MCP retry duplication
- **Named `fact_*` prefix** — distinct from mempalace's `kg_*` naming

### Schema

File: `~/.memex/knowledge_graph.db`

```sql
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

CREATE INDEX IF NOT EXISTS idx_facts_subject ON facts(subject);
CREATE INDEX IF NOT EXISTS idx_facts_object ON facts(object);
CREATE INDEX IF NOT EXISTS idx_facts_subject_predicate ON facts(subject, predicate);
CREATE INDEX IF NOT EXISTS idx_facts_valid_until ON facts(valid_until);
```

### Go Implementation

New file: `internal/kg.go`

```go
type KnowledgeGraph struct {
    db *sql.DB
}

func NewKnowledgeGraph(path string) (*KnowledgeGraph, error)
func (kg *KnowledgeGraph) Init() error                    // CREATE TABLE + indexes + WAL
func (kg *KnowledgeGraph) RecordFact(subject, predicate, object, validFrom, source string, singular bool) (string, error)
func (kg *KnowledgeGraph) ExpireFact(subject, predicate, object, ended string) error
func (kg *KnowledgeGraph) QueryEntity(entity, asOf string) ([]Fact, error)
func (kg *KnowledgeGraph) History(entity string) ([]Fact, error)
func (kg *KnowledgeGraph) Stats() (KGStats, error)
```

### HTTP Endpoints (for UI)
- `POST   /facts`                         — record a fact
- `GET    /facts?subject=X&as_of=Y`       — query entity
- `DELETE /facts/:id`                     — expire a fact
- `GET    /facts/timeline?entity=X`       — chronological history
- `GET    /facts/stats`                   — overview

### Docker / File Changes
- `docker-compose.yml`: `~/.memex:/root/.memex` volume (same mount as L0 identity file)
- Go module: add `modernc.org/sqlite` (pure Go, no CGO required)

---

## Sub-project 4: Transcript Mining

### What mempalace does
`mempalace mine <dir>` CLI with 3 modes: `projects`, `convos`, `general`. Pure regex classification via `general_extractor.py` — 5 marker sets, speaker-turn splitting, disambiguation logic, minimum confidence threshold. Always a manual deliberate action. Has `mempalace split` to break concatenated mega-files first.

### What memex does
`transcript.go` parses Claude Code JSONL and extracts reasoning strings only — used solely to attach reasoning to trace events in the UI. Does not classify, does not save memories, does not mine. Session-stop hook receives `transcript_path` and does nothing with it for memory purposes.

### What mempalace does better
- Mines full conversation turns (both user and assistant) — user turns carry the most valuable preferences and decisions
- Speaker-turn splitting is semantically meaningful (conversation boundaries)
- Deduplication check before filing
- Deliberate re-runnable CLI — can mine the same directory multiple times safely

### How we improve
- **9-type classifier in Go** — extends mempalace's 5 types with 4 new marker sets (`context`, `procedure`, `rationale`, and we rename `milestone` → `discovery` to match our taxonomy)
- **Mine both sides** — extend `ParseTranscript` to return full conversation turns (user + assistant), not just reasoning. User turns are where "I prefer X", "we decided Y", "the bug was Z" actually appear
- **Automatic on session-stop** — mempalace is always manual. memex fires `POST /mine/transcript` asynchronously in the session-stop hook. Zero user action required
- **Topic inference** — auto-infer `topic` from dominant noun slugs in the segment. Less manual overhead than mempalace's explicit `wing`/`room` requirement
- **Duplicate detection on import** — before saving a mined memory, embed it and check cosine similarity. If similarity > 0.92 against an existing memory for the same project, skip

### Classifier Marker Sets (Go)

```go
var typeMarkers = map[string][]string{
    "decision":   {"let's use", "we decided", "went with", "rather than", "architecture", "approach", "strategy", "configure", "default", "trade-off"},
    "preference": {"i prefer", "always use", "never use", "don't use", "i like", "i hate", "my rule", "we always", "we never", "snake_case", "camel_case"},
    "event":      {"session on", "we met", "sprint", "release", "last week", "deployed", "shipped", "launched", "milestone", "standup"},
    "discovery":  {"it works", "figured out", "turns out", "the trick is", "realized", "breakthrough", "finally", "now i understand", "the key is"},
    "advice":     {"you should", "recommend", "best practice", "the answer is", "suggestion", "consider", "try using", "better to"},
    "problem":    {"bug", "error", "crash", "doesn't work", "root cause", "the fix", "workaround", "broken", "failing", "issue"},
    "context":    {"works on", "responsible for", "reports to", "team", "owns", "based in", "member of", "leads"},
    "procedure":  {"steps to", "how to", "workflow", "always run", "first run", "then run", "pipeline", "process"},
    "rationale":  {"the reason we", "chose over", "trade-off", "we rejected", "instead of", "because we need", "pros and cons"},
}
```

Disambiguation rules (ported from mempalace):
- Problem + resolution markers → `discovery`
- Problem + positive sentiment → `discovery`

### New Components

**`internal/classifier.go`** — `Classifier` type with `Classify(text string) (memoryType string, confidence float64)`

**`internal/miner.go`** — `Miner` type:
```go
func (m *Miner) MineTranscript(path, project string) ([]SaveMemoryRequest, error)
func (m *Miner) inferTopic(text string) string
```

**Extended `ParseTranscript`** — returns `[]ConversationTurn{Role, Text}` instead of `[]string`

**New CLI subcommand:** `memex mine <path>` — scans path, mines memories, POSTs to `localhost:8765`

**New HTTP endpoint:** `POST /mine/transcript` — body: `{path, project}`, runs miner in goroutine, returns `{status: "mining started", path: "..."}`

### Session-Stop Hook Change

```go
func hookSessionStop() {
    // existing: POST /trace/stop
    // NEW: POST /mine/transcript (async, non-blocking)
    if input.TranscriptPath != "" {
        go http.Post(getMemexURL()+"/mine/transcript", ...)
    }
}
```

---

## Sub-project 5: MCP Tools

### What mempalace does
19 tools across 5 categories. `mempalace_status` teaches Claude the entire protocol on first call — wings, rooms, halls, AAAK spec, when to search, when to save, when to invalidate. Claude is self-orienting. Duplicate detection before filing. Type + topic filtered search. Full KG access. Navigation graph tools.

### What memex does
4 tools: `save_memory`, `search_memory`, `list_memories`, `delete_memory`. No schema guidance, no taxonomy visibility, no duplicate detection, no type filtering, no KG access. Claude saves blindly with no orientation.

### What mempalace does better
- Self-describing protocol — Claude learns the system from the system itself
- Taxonomy visibility before writing — Claude knows what's already stored
- Duplicate detection — prevents noise accumulation
- KG tools — structured fact management that vector search can't do
- Type + topic scoped search — 34% better retrieval starts at the tool level

### How we improve
- **`memory_overview`** injects the memex protocol directly in the tool description — Claude learns the 9 types, when to use KG vs Qdrant, when to pin, on first call
- **`find_similar`** — duplicate check before every save, same philosophy as mempalace but embedded in the workflow
- **`fact_*` prefix** — clearly groups KG tools as a family, distinct from mempalace's `kg_*`
- **`pin_memory`** — no mempalace equivalent. Promotes any memory to L1 (importance = 1.0) so it's always loaded on session-start. Gives Claude a way to say "this matters forever"
- **`digest_session`** — triggers transcript mining from within a Claude session. Lets Claude mine a past session on demand without the CLI

### 13 MCP Tools

**Memory (6):**

| Tool | Description |
|---|---|
| `save_memory` | Save a memory. Requires `memory_type` (one of 9). Accepts `topic` slug. |
| `search_memory` | Semantic search. Accepts `memory_type`, `topic`, `project` filters. |
| `list_memories` | List memories. Accepts `memory_type`, `topic`, `project` filters. |
| `delete_memory` | Delete by ID. |
| `find_similar` | Embed candidate text, return top matches with similarity scores. Call before `save_memory` to check for duplicates. |
| `memory_overview` | Returns: taxonomy (project→topic→type→count), total count, KG stats, and the **memex protocol** — when to save, which type to use, when to use KG vs Qdrant. Self-describing. |

**Knowledge Graph (5):**

| Tool | Description |
|---|---|
| `fact_record` | Add a triple: subject, predicate, object, valid_from, source, singular. If `singular=true`, closes any existing active fact for same subject+predicate first. |
| `fact_query` | Current facts about an entity. Optional `as_of` date for point-in-time queries. |
| `fact_expire` | Close a fact's validity window (sets valid_until). Fact is preserved for history. |
| `fact_history` | Chronological timeline of all facts for an entity. |
| `fact_stats` | KG overview: entity count, triple count, active vs expired, relationship types. |

**Pinned (1):**

| Tool | Description |
|---|---|
| `pin_memory` | Set importance = 1.0 on a memory by ID, promoting it to L1 (always loaded on session-start). |

**Mining (1):**

| Tool | Description |
|---|---|
| `digest_session` | Trigger transcript mining for a given path. Runs asynchronously. Returns immediately. |

### memex Protocol (injected via `memory_overview`)

```
memex Memory Protocol:
1. ON SESSION START: memory context is pre-loaded. No need to call memory_overview immediately.
2. BEFORE SAVING: call find_similar first. If similarity > 0.92, skip or update instead.
3. CHOOSING memory_type:
   - decision: architecture choices, locked-in resolutions
   - preference: coding style, tool choices, habits (use pin_memory for critical ones)
   - event: deployments, milestones, sessions
   - discovery: breakthroughs, "it works" moments, key insights
   - advice: recommendations, best practices, solutions
   - problem: bugs, errors, root causes + fixes
   - context: team members, org structure, project relationships
   - procedure: workflows, build steps, repeatable processes
   - rationale: WHY a decision was made, trade-offs considered
4. USE KG (fact_*) FOR: named entities, relationships, facts that change over time
   USE Qdrant (save_memory) FOR: unstructured knowledge, explanations, context blobs
5. PINNING: call pin_memory on any memory that must survive every session-start
6. WHEN FACTS CHANGE: call fact_expire on the old fact, fact_record for the new one
```

---

## Infrastructure Changes Summary

### docker-compose.yml
```yaml
services:
  memex:
    volumes:
      - ~/.memex:/root/.memex   # NEW: host mount for identity.md + knowledge_graph.db
```

### New files
```
internal/kg.go           — KnowledgeGraph type (SQLite WAL)
internal/classifier.go   — 9-type regex classifier
internal/miner.go        — transcript miner
~/.memex/identity.md     — L0 identity (user-created)
~/.memex/knowledge_graph.db  — SQLite KG (auto-created on startup)
```

### Modified files
```
internal/models.go       — Memory + SaveMemoryRequest gain topic, memory_type
internal/store.go        — Store interface updated (new filter params)
internal/qdrant.go       — schema migration, new indexes, filter support
internal/handlers.go     — new params, new endpoints (/memories/pinned, /mine/transcript, /facts/*)
internal/mcp.go          — 13 tools replacing 4
internal/hook.go         — L0+L1+L2 session-start, async mining on session-stop
internal/server.go       — register new routes, init KG alongside Qdrant
internal/transcript.go   — return full conversation turns not just reasoning strings
internal/config.go       — IdentityPath, KGPath config fields
docker-compose.yml       — host volume mount
go.mod                   — add modernc.org/sqlite
```

### Go module additions
- `modernc.org/sqlite` — pure Go SQLite driver (no CGO)

---

## What We Explicitly Do NOT Build (YAGNI)

- AAAK compression — mempalace's raw mode outperforms AAAK. We store raw text.
- Agent diaries — out of scope for v2
- Navigation graph tools (`traverse`, `find_tunnels`) — KG tools serve cross-entity discovery
- Multi-format mining (Slack, ChatGPT exports) — Claude Code JSONL only for v2
- `split` command for mega-files — not needed for Claude Code transcripts
- Importance decay — future v3 work
- Stale memory cleanup — future v3 work
