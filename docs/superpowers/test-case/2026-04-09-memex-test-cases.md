# memex v2 — Detailed Use Cases & Test Design Specification

**Purpose:** This document translates the v2 architecture into **concrete product use cases, behavior expectations, edge cases, and test scenarios**. It is designed to directly support **unit tests, integration tests, MCP contract tests, hook tests, and end-to-end workflow validation**.

This is intentionally written from a **behavior-first perspective** so implementation can be validated against user-facing outcomes instead of only internal function correctness.

---

# 1) Core Product Goals

memex v2 should reliably support five product jobs:

1. **Recall important knowledge quickly at session start**
2. **Store structured long-term learnings from active work**
3. **Track changing facts over time without losing history**
4. **Mine previous conversations into reusable knowledge automatically**
5. **Help Claude choose the right memory primitive (vector vs fact)**

All use cases below map back to one or more of these goals.

---

# 2) Use Case Matrix

| Use Case                   | Primary Layer     | Main API / Tool            | Test Type   |
| -------------------------- | ----------------- | -------------------------- | ----------- |
| Session wake-up            | L0/L1/L2          | hook session-start         | integration |
| Save architecture decision | Structured memory | save_memory                | unit + MCP  |
| Save coding preference     | pinned memory     | save_memory + pin_memory   | integration |
| Query current truth        | KG                | fact_query                 | unit        |
| Query historical truth     | KG timeline       | fact_history               | unit        |
| Replace singular fact      | KG                | fact_record(singular=true) | unit        |
| Mine transcript            | miner             | digest_session             | integration |
| Prevent duplicates         | similarity        | find_similar               | unit        |
| Session-stop learning      | hook + miner      | session-stop               | e2e         |
| Trace-to-memory checkpoint | trace + memory    | checkpoint                 | integration |

---

# 3) Session Start Use Cases

## 3.1 Identity bootstrapping

### User story

When a new Claude Code session starts, the system should preload stable identity context.

### Expected behavior

* Read `~/.memex/identity.md`
* If file exists, inject under `[identity]`
* If missing, skip silently
* No network dependency

### Test cases

### Happy path

* identity file exists
* hook output contains `[identity]`
* exact file content preserved

### Edge cases

* empty file
* large file (>5KB)
* malformed unicode
* missing directory
* permission denied

### Assertions

* no panic
* missing file does not fail session-start
* output block remains valid

---

## 3.2 Pinned project recall

### User story

Critical project preferences and decisions must always appear.

### Expected behavior

* fetch memories where `importance >= 0.9`
* filter by active project
* sort by importance desc
* max 10

### Test cases

* multiple projects mixed
* only current project returned
* exactly 10 returned when >10 exist
* no pinned memories returns empty `[pinned]`

### Edge cases

* importance exactly 0.9
* null project
* store unavailable

---

## 3.3 Semantic context recall

### User story

Relevant context from recent project work should supplement pinned memory.

### Expected behavior

* semantic search scoped to project
* limit 5
* prioritize `preference` and `decision`

### Test cases

* mixed memory types
* preference should rank above event with similar score
* empty search results
* semantic failure fallback

---

# 4) Structured Memory Use Cases

## 4.1 Save architecture decision

### Example

“Use SQLite WAL for temporal KG because single writer + historical queries.”

### Expected behavior

* memory_type = `decision`
* topic inferred or provided = `knowledge-graph`
* project required
* searchable by project + type + topic

### Tests

* valid decision saves successfully
* invalid type rejected
* empty text rejected
* topic defaults to project

### Assertions

* Qdrant payload includes `memory_type`
* `topic` indexed
* retrievable via `memory_type=decision`

---

## 4.2 Save rationale separately

### User story

A rationale should preserve *why* a choice happened.

### Expected behavior

* memory_type = `rationale`
* linked by shared topic
* appears in decision-oriented retrieval

### Tests

* same topic as decision
* rationale-only query
* decision + rationale combined listing

---

## 4.3 Save repeatable workflow

### Example

“Always run distill before mining old transcripts.”

### Expected behavior

* memory_type = `procedure`
* high retrieval priority for how-to prompts

### Tests

* saved under procedure
* filtered retrieval by type
* pinning promotes workflow to L1

---

# 5) Duplicate Prevention Use Cases

## 5.1 Manual save duplicate detection

### User story

Claude should not save nearly identical learnings repeatedly.

### Expected behavior

* `find_similar` called before save
* similarity > 0.92 triggers skip

### Tests

* exact duplicate
* paraphrased duplicate
* cross-project same text should NOT skip
* threshold boundary 0.919 vs 0.921

### Assertions

* duplicate not persisted
* save response indicates skipped reason

---

## 5.2 Transcript mining duplicate suppression

### Expected behavior

Mined discoveries from repeated sessions should not multiply.

### Tests

* same transcript mined twice
* similar turns across two sessions
* repeated checkpoint summary

---

# 6) Knowledge Graph Use Cases

## 6.1 Record stable current fact

### Example

`memex -> uses -> qdrant`

### Expected behavior

* fact inserted active
* valid_until null
* query current returns fact

### Tests

* insert then query
* object-based reverse query
* idempotent reinsertion

---

## 6.2 Replace singular fact

### Example

`memex uses sqlite` → later `memex uses qdrant`

### Expected behavior

* old fact closed automatically
* new fact active
* timeline contains both in order

### Critical tests

* singular=false keeps both active
* singular=true closes previous
* repeated singular write same object is idempotent

### Assertions

* only one active `(subject,predicate)`
* historical interval preserved

---

## 6.3 Historical query

### User story

Ask what was true on a prior date.

### Example

“What database did memex use on 2026-04-01?”

### Tests

* before replacement date
* exact replacement boundary
* after replacement
* no matching fact

---

## 6.4 Explicit fact expiry

### Expected behavior

DELETE route should set `valid_until`, not delete row.

### Tests

* expire active fact
* expire already expired fact
* invalid id
* explicit ended date

---

# 7) Transcript Mining Use Cases

## 7.1 Mine decisions from conversation

### Example turn

“We decided to keep tracing as memex’s moat.”

### Expected behavior

* classifier → decision
* confidence > threshold
* topic inferred `tracing`
* saved once

### Tests

* user-side decision
* assistant-side decision
* mixed multi-turn decision

---

## 7.2 Mine discoveries from resolved problems

### Example

“The issue was host.docker.internal instead of localhost — now it works.”

### Expected behavior

* initially scores as problem
* disambiguation upgrades to discovery

### Tests

* problem markers + resolution phrase
* positive sentiment promotion
* unresolved issue remains problem

---

## 7.3 Mine procedures

### Example

“First run docker compose, then start Ollama, then run memex serve.”

### Tests

* ordered steps
* code block mixed with prose
* short snippets ignored (<50 chars)

---

# 8) Session Stop Learning Loop

## 8.1 Automatic async mining

### User story

When session ends, learnings should persist without manual action.

### Expected behavior

* trace stop executes first
* mining POST runs async
* hook exits immediately

### Tests

* transcript path present
* transcript path missing
* miner service down
* invalid file path

### Assertions

* trace stop still succeeds if mining fails
* no hook latency regression

---

# 9) MCP Tooling Use Cases

## 9.1 Self-orientation

### User story

Claude should learn the protocol from the system.

### Expected behavior

`memory_overview` returns:

* taxonomy
* total memories
* KG stats
* protocol rules

### Tests

* empty store
* populated multi-project store
* KG unavailable

---

## 9.2 Promote critical memory

### User story

A highly important preference should become always-on context.

### Flow

* save_memory(preference)
* pin_memory(id)
* verify session-start returns it

### E2E assertion

Appears in `[pinned]`

---

# 10) Trace + Memory Convergence Use Cases

## 10.1 Checkpoint summary persistence

### User story

A successful coding checkpoint should become an `event` memory.

### Expected behavior

* summary saved
* topic = checkpoint
* importance = 0.9
* pinned endpoint includes it

### Tests

* checkpoint with empty summary
* repeated checkpoint dedupe
* cross-session checkpoint ordering

---

## 10.2 Trace-informed mining confidence

### Future-facing tests

Even before implementation, define expected behavior:

* successful tool sequence increases mining confidence
* repeated failures bias toward `problem`
* resolved failure after success promotes `discovery`

This will help v3 evolution.

---

# 11) Failure Mode Test Plan

## Storage failures

* Qdrant unavailable
* SQLite locked
* WAL corruption recovery
* host mount missing

## Network failures

* MCP timeout
* hook HTTP timeout
* partial response body

## Data failures

* malformed transcript JSONL
* invalid UTF-8
* invalid dates
* huge payloads

## Consistency failures

* duplicate fact race
* repeated mining on same transcript
* pinning deleted memory

---

# 12) Recommended Test Suite Structure

```text
internal/
  classifier_test.go
  miner_test.go
  kg_test.go
  hook_test.go
  handlers_test.go
  mcp_test.go
  integration/
    session_start_test.go
    session_stop_test.go
    mining_pipeline_test.go
    fact_timeline_test.go
    checkpoint_flow_test.go
```

---

# 13) Highest ROI Test Order

Implement tests in this order:

1. KG singular replacement
2. session-start layering
3. transcript mining classifier
4. duplicate suppression
5. session-stop async mining
6. checkpoint → event memory
7. MCP protocol overview

This order validates the **highest-risk correctness paths first**.

---

# 14) Final Validation Goal

The product is correct when it can reliably support this loop:

> session starts with right context → user works → traces captured → decisions learned → facts updated → next session starts smarter

That closed learning loop is the defining behavioral success criteria for memex v2.

---

# 15) Deep Technical Test Design (Implementation-Level)

This section converts the behavioral use cases into **low-level technical test contracts**, fixture design, dependency seams, concurrency validation, and deterministic assertions.

---

## 15.1 Store Contract Test Matrix

Every `Store` implementation (currently Qdrant) must pass the same reusable contract suite.

### Required contract invariants

* `SaveMemory` must return a non-empty stable ID
* `Timestamp <= LastAccessed` on initial save
* `Topic` defaults to `Project` when omitted
* invalid `memory_type` must never reach store layer
* `ListMemories` must be deterministic under equal scores
* `PinnedMemories(project)` must only return `importance >= 0.9`
* `FindSimilar` must preserve descending similarity order

### Suggested reusable test harness

```text
func RunStoreContractTests(t *testing.T, storeFactory func() Store)
```

Run against:

* real Qdrant in Docker integration
* in-memory fake store for unit tests
* future Postgres adapter if introduced

---

## 15.2 Qdrant Payload Integrity Tests

These tests validate exact payload serialization and filter generation.

### Save payload assertions

For every save:

* `text` stored as string
* `project` keyword field present
* `memory_type` keyword field present
* `topic` keyword field present
* `importance` numeric float payload
* `timestamp` RFC3339
* `last_accessed` RFC3339

### Search filter assertions

Construct golden tests for:

* project only
* project + type
* project + topic
* project + type + topic

Assert exact Qdrant `must` clauses count.

### Failure-mode cases

* missing vector index
* dimension mismatch from Ollama
* malformed payload type coercion

---

## 15.3 SQLite KG Determinism Tests

KG correctness is the most important technical surface.

### Core invariants

1. `RecordFact(singular=true)` leaves max 1 active `(subject,predicate)`
2. `ExpireFact` never deletes rows
3. `History` sorted by `valid_from ASC`
4. identical active triple is idempotent
5. `QueryEntity(as_of)` respects interval boundaries exactly

### Boundary test cases

#### replacement boundary

* old valid_until = `2026-04-10T10:00:00Z`
* query at exact timestamp
* define strict expected semantics:

  * closed interval exclusive on `valid_until`
  * active if `valid_until > as_of`

This must be frozen in tests.

### WAL concurrency tests

Use goroutines:

* 1 writer loop replacing singular facts
* 20 concurrent readers querying current truth
* assert no reader errors
* assert final active fact count == 1

This specifically validates WAL assumptions.

---

## 15.4 Transcript Fixture Design

Create reusable Claude Code JSONL fixtures.

### Fixture categories

```text
fixtures/transcripts/
  decision_single_turn.jsonl
  discovery_resolution.jsonl
  procedure_multistep.jsonl
  noisy_debug_loop.jsonl
  malformed_line.jsonl
  duplicate_session.jsonl
```

### Fixture validation assertions

For each fixture assert:

* parsed turn count
* role preservation
* tool_use blocks excluded
* multiline text blocks merged predictably
* malformed lines fail fast with line number

This is critical because mining quality depends entirely on parser determinism.

---

## 15.5 Classifier Technical Accuracy Tests

The classifier should be table-driven with **marker ambiguity cases**.

### Suggested structure

```text
name
input
expectedType
minConfidence
```

### High-value ambiguity cases

* problem + solved → discovery
* rationale vs decision overlap
* advice vs procedure overlap
* context vs decision (“X owns Y system design”)
* positive debugging statement with error words

### Noise robustness tests

Input contains:

* stack traces
* code blocks
* shell commands
* markdown bullets
* quoted transcript snippets

Assert `extractProse()` strips noise deterministically.

---

## 15.6 Duplicate Detection Threshold Tests

The `0.92` threshold is a product-critical heuristic.

### Technical tests

Mock `FindSimilar` results with scores:

* 0.919 → should save
* 0.920 → define exact boundary behavior
* 0.921 → skip
* same text different project → save
* different topic same project high similarity → configurable expected behavior

Freeze this logic because threshold drift will silently change memory growth.

---

## 15.7 Hook Golden Output Tests

Session-start hook should use **golden snapshot tests**.

### Golden files

```text
fixtures/golden/
  session_start_identity_only.txt
  session_start_full_layers.txt
  session_start_offline.txt
```

### Assertions

* exact section ordering: identity → pinned → context
* stable newline formatting
* no duplicate bullets
* empty sections omitted or retained based on spec
* token count stays under expected budget

This protects UX regressions.

---

## 15.8 MCP Contract Tests

Every MCP tool should be validated as an API contract, not only handler logic.

### Required assertions

* parameter schema correctness
* required fields enforced
* enum validation for `memory_type`
* KG tool naming prefix consistency
* error propagation preserves actionable message

### Example critical flows

#### save workflow

`find_similar -> save_memory -> pin_memory`

#### fact workflow

`fact_query -> fact_record(singular) -> fact_history`

The exact tool chaining expectations should be codified.

---

## 15.9 Async Mining Race Tests

This is one of the highest-risk technical paths.

### Scenario

`hookSessionStop` triggers:

1. trace stop
2. async mining goroutine
3. temp cleanup

### Test requirements

* hook returns before miner completes
* temp file cleanup does not delete transcript dependency too early
* repeated session-stop calls on same transcript are idempotent
* miner panic does not affect trace completion

Use synchronization primitives:

* `sync.WaitGroup`
* buffered channels
* fake delayed HTTP server

---

## 15.10 End-to-End Deterministic Replay Tests

The strongest system test is **replay determinism**.

### Replay scenario

Given:

* fixed transcript fixture
* fixed clock
* fixed UUID generator
* fake embedding vector

Run full flow twice:

1. session-start
2. save decisions
3. session-stop mining
4. restart session

### Assertions

Second replay should produce:

* identical KG state
* identical memory counts
* no duplicate discoveries
* same pinned output order
* same taxonomy counts

This guarantees reproducibility.

---

# 16) Benchmark & Performance Test Plan

Even though scale is small, benchmark critical loops.

## Benchmarks

* `BenchmarkClassifierLargeTranscript`
* `BenchmarkKGSingularReplacement`
* `BenchmarkPinnedMemories1000`
* `BenchmarkMineDirectory100Sessions`
* `BenchmarkSessionStartHook`

### Performance budgets

* session-start hook: < 150ms local
* singular fact replace: < 5ms p50
* transcript mine 1 session: < 500ms excluding embeddings
* duplicate check: < 100ms local Ollama

Freeze budgets in CI trend monitoring.

---

# 17) Recommended Mocking Strategy

Use explicit interfaces for:

* clock
* UUID generator
* embedding client
* Qdrant HTTP client
* transcript filesystem
* KG database opener

This enables deterministic tests for:

* timestamps
* ordering
* replacement boundaries
* retry/idempotency

Without this, your historical timeline tests will become flaky.

---

# 18) Technical Definition of Done

Implementation is technically complete only when:

* all store contracts pass
* WAL concurrency test passes reliably
* transcript replay deterministic
* golden hook snapshots stable
* duplicate threshold boundaries frozen
* full session learning loop replay is idempotent
* benchmarks within budget

This transforms memex v2 from feature-complete into **production-correct and regression-safe**.
