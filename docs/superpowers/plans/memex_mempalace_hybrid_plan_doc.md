# Memex × MemPalace Hybrid — Implementation Plan Doc

## Vision
Build a **developer-friendly memory navigation experience** inspired by MemPalace’s spatial UX, while preserving **Memex’s temporal DAG + lineage-first storage core**.

**Guiding principle:**
> UX should feel like exploring a memory palace.
>
> Truth should remain an event-sourced temporal graph.

This gives the product:
- intuitive exploration
- replayable reasoning
- temporal debugging
- branch-aware memory
- better agent observability
- strong OSS storytelling

---

# 1) Product thesis

## Problem
Current memory systems optimize for:
- semantic similarity
- transcript recall
- prompt stuffing

But they fail at:
- **why a decision happened**
- **how context evolved over time**
- **which branch led to the final outcome**
- **how to replay reasoning state**

## Proposed solution
Split the system into:

### A) Navigation projection layer
Human-friendly abstraction:
- Wings
- Rooms
- Halls
- Tunnels
- Closets
- Drawers
- **Time Corridors** (new)

### B) Temporal graph truth layer
Canonical source:
- sessions
- chunks
- manifests
- decisions
- actions
- outcomes
- lineage DAG
- branch graph
- validity windows

---

# 2) Architecture blueprint

```text
┌──────────────────────────────┐
│ Human / Agent Query Surface  │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│ Palace Projection Layer      │
│ virtual UX filesystem        │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│ Retrieval Planner            │
│ semantic + temporal + DAG    │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│ Memex Temporal Graph Core    │
│ SQLite + lineage + replay    │
└──────────────────────────────┘
```

---

# 3) Core abstractions mapping

## UX → graph mapping

| UX abstraction | Backing storage concept |
|---|---|
| Wing | namespace / workspace |
| Room | topic cluster / graph community |
| Hall | edge type / memory class |
| Tunnel | lineage edge traversal |
| Closet | summary snapshot materialized view |
| Drawer | replayable raw session thread |
| Time Corridor | timeline slice + temporal filter |

---

# 4) Data model plan

## Core tables

### sessions
- session_id
- namespace
- started_at
- actor
- branch_id

### chunks
- chunk_id
- session_id
- manifest_id
- content
- embedding_ref
- created_at

### lineage_edges
- from_node
- to_node
- edge_type
- confidence
- created_at

### decisions
- decision_id
- chunk_id
- rationale
- chosen_branch
- timestamp

### actions
- action_id
- decision_id
- action_type
- state_diff
- timestamp

### outcomes
- outcome_id
- action_id
- status
- result_summary
- observed_at

### summary_snapshots
- snapshot_id
- scope_key
- summary
- valid_from
- valid_to

---

# 5) Milestone roadmap

## Milestone 1 — Temporal DAG foundation (Week 1)
### Goal
Stabilize Memex as truth layer.

### Deliverables
- SQLite schema finalized
- lineage_edges table
- branch_id support
- validity windows
- manifest ancestry
- replay_group_id

### Success metric
Can answer:
> what led to this decision?

---

## Milestone 2 — Palace projection engine (Week 2)
### Goal
Generate virtual memory palace from graph metadata.

### Deliverables
- wing generator from namespace
- room generator from topic communities
- hall generator from edge taxonomy
- tunnel generator from lineage paths
- time corridor generator from timestamps

### API shape
```json
{
  "wing": "payments",
  "rooms": ["retries", "webhooks"],
  "corridors": ["2026-03", "2026-04"]
}
```

### Success metric
Every graph slice maps to a navigable UX path.

---

## Milestone 3 — Navigation DSL (Week 3)
### Goal
Make navigation feel like filesystem traversal.

### DSL examples
```text
/payments/retries/incidents
/payments/retries@2026-03
/payments/retries#lineage
```

### Deliverables
- parser
- path resolver
- graph query translator
- MCP-compatible command surface
- CLI explorer

### Success metric
Agent can traverse memory with symbolic paths.

---

## Milestone 4 — Replay drawers (Week 4)
### Goal
Turn raw sessions into explainable replay threads.

### Deliverables
- session replay engine
- prompt → retrieval → response → action chain
- state diff snapshots
- branch replay support
- merge visualization

### Success metric
Can replay entire architecture evolution.

---

## Milestone 5 — Smart retrieval planner (Week 5)
### Goal
Beat vector-only memory systems.

### Pipeline
1. namespace narrowing
2. room clustering
3. time corridor filter
4. lineage neighborhood expansion
5. semantic rerank
6. summary snapshot fallback

### Deliverables
- hybrid scorer
- lineage confidence ranking
- temporal decay scoring
- replay relevance score

### Success metric
Higher precision on decision-history queries.

---

# 6) Killer differentiators

## A) Time corridors
Unique Memex UX innovation.

Users should navigate:
```text
payments → retries → Mar 2026 corridor
```

This makes memory evolution visible.

---

## B) Replay drawers
This is the moat.

```text
open /payments/retries/incident-42
```

Should replay:
- initial issue
- retrieved memory
- decision tree
- action chain
- rollback
- final stable state

This becomes:
> Git blame + time travel for AI memory

---

## C) Branch merge memory
Preserve explored alternatives.

Example:
- retry strategy A
- retry strategy B
- queue-first fallback

Merged branch keeps rationale history.

---

# 7) OSS packaging plan

## Developer story
Pitch as:
> spatial UX for temporal memory graphs

## README structure
- problem
- architecture diagram
- palace navigation demo
- replay GIF
- SQLite schema
- MCP integration
- benchmark vs vector memory

## Demo flow
Best demo:
```text
show wing payments
walk retries room
open Mar corridor
replay outage drawer
```

This will be highly memorable.

---

# 8) Success criteria
The hybrid is successful if it can answer:

1. **What changed since last month?**
2. **Why did we choose branch B?**
3. **Replay the reasoning behind v3.**
4. **Show incidents connected to this design.**
5. **Which summary snapshot is stale?**

If these are easy, the architecture is working.

---

# 9) Final recommendation
Do **not** store the palace literally.

Instead:
> palace = dynamic projection
>
> graph = canonical truth

That separation is what keeps the system:
- scalable
- replayable
- debuggable
- agent-native
- impossible to outgrow

This is the right path to make Memex feel magical **without sacrificing systems depth**.



---

# 10) Detailed phased plan — Obsidian × Memex integration

## Goal
Turn Obsidian into the **human-facing cognition layer** for Memex while preserving Memex as the **canonical temporal graph and replay engine**.

**Core principle**
> Obsidian stores projections.
>
> Memex stores truth.

This separation prevents markdown from becoming the system of record while still leveraging Obsidian’s graph UX, backlinks, Canvas, and plugin ecosystem.

---

## Phase 0 — UX contract and information architecture (Week 0)
### What we are doing
Define the **Memex-native navigation vocabulary inside Obsidian**.

Standardize:
- Spaces → folders
- Threads → primary notes
- Bridges → wikilinks
- Snapshots → generated time-slice notes
- Replays → markdown logs + Canvas files
- Timelines → folder/date partition conventions

Define note templates and path semantics.

### Why we are doing it
Without a stable UX contract, later sync and plugin work becomes brittle.

This phase ensures:
- predictable vault layout
- deterministic sync targets
- stable wikilink generation
- plugin command consistency
- README and demo clarity

### Deliverables
- vault path specification
- note naming RFC
- snapshot naming rules
- canvas naming rules
- replay template schema

### Success criteria
A user can understand the entire memory model by looking only at the vault tree.

---

## Phase 1 — Projection sync engine MVP (Week 1)
### What we are doing
Build a **one-way sync adapter**:

```text
Memex graph → markdown projection → Obsidian vault
```

The sync engine should:
- read graph slices
- render markdown notes
- emit wikilinks from bridge edges
- write snapshots as dated files
- preserve stable note IDs
- support incremental sync

### Why we are doing it
This is the fastest way to validate product value **without building a plugin first**.

Users can immediately:
- browse threads
- use graph view
- search notes
- backlink decisions
- inspect timeline evolution

This creates fast OSS adoption.

### Deliverables
- CLI command
```text
memex obsidian sync --vault <path>
```
- note renderer
- bridge wikilink generator
- file diff updater
- deleted-note archival strategy

### Example output
```text
vault/
 └── payments/
      ├── retries.md
      ├── retries@2026-03.md
      └── reconciliation.md
```

### Success criteria
A full project space can be synced into a readable vault in under 3 seconds.

---

## Phase 2 — Live thread note system (Week 2)
### What we are doing
Introduce **always-fresh canonical thread notes**.

Each thread note becomes a materialized live view containing:
- latest summary
- key decisions
- recent bridges
- latest replay references
- timeline jumps
- stale state warnings

### Why we are doing it
This becomes the **daily working surface** for users.

Instead of opening many files, users open one thread note and immediately understand:
- current state
- why it changed
- what happened recently
- what to replay next

This dramatically improves usability.

### Deliverables
- thread note template
- latest summary materializer
- recent event feed
- stale snapshot detector
- latest bridge ranking

### Template
```md
# Payment Retries

## Current Snapshot
...

## Latest Decisions
...

## Recent Bridges
- [[queue-redesign]]

## Replays
- [[rollback.canvas]]
```

### Success criteria
Users can recover full project context from a single note within 30 seconds.

---

## Phase 3 — Timeline and snapshot system (Week 3)
### What we are doing
Build **time-native vault projections**.

Each thread gets temporal slices:
```text
retries@2026-03.md
retries@2026-04.md
```

Optionally organize by timeline folders:
```text
payments/retries/timeline/2026-04.md
```

### Why we are doing it
Temporal evolution is Memex’s moat.

Obsidian users already think in:
- daily notes
- weekly reviews
- monthly logs

This phase makes Memex feel deeply native to existing Obsidian workflows.

### Deliverables
- time slicing engine
- temporal summary generator
- monthly / weekly snapshot strategies
- timeline backlinks
- change diff section

### Success criteria
Users can answer “what changed this month?” directly from vault notes.

---

## Phase 4 — Replay rendering engine (Week 4)
### What we are doing
Generate **human-readable replay notes** from lineage DAG paths.

Replay note structure:
- initial trigger
- retrieved memory
- decisions made
- actions executed
- state mutations
- final outcome
- branch alternatives

### Why we are doing it
Replay is the strongest Memex differentiator.

Obsidian makes this incredibly useful for:
- postmortems
- architecture evolution
- interview prep
- PR retrospectives
- incident debugging

### Deliverables
- replay markdown renderer
- branch merge sections
- decision rationale blocks
- state diff formatter
- replay index page

### Success criteria
Users can replay any major system decision from the vault without touching SQL.

---

## Phase 5 — Canvas DAG visualizer (Week 5)
### What we are doing
Generate native **Obsidian Canvas files** for replay graphs.

Render nodes as:
- prompts
- retrieved chunks
- decision points
- actions
- outcomes
- rollbacks

Edges represent lineage and causal flow.

### Why we are doing it
Canvas transforms replay from logs into **visual reasoning maps**.

This dramatically improves:
- incident review
- design walkthroughs
- debugging clarity
- demo virality

This phase creates the “wow factor.”

### Deliverables
- `.canvas` serializer
- DAG → canvas layout engine
- replay canvas export command
- stable node positioning heuristics

### Success criteria
A complex outage replay is visually understandable in under 60 seconds.

---

## Phase 6 — Native Obsidian plugin (Week 6–7)
### What we are doing
Build a plugin so users can interact with Memex **without leaving Obsidian**.

Plugin commands:
- Open Space
- Refresh Thread
- Jump Timeline
- Generate Snapshot
- Replay Current Thread
- Show Bridges
- Explain Current Note Lineage

### Why we are doing it
This removes CLI friction and turns Memex into a **first-class Obsidian intelligence layer**.

This is the adoption multiplier.

### Deliverables
- TypeScript plugin
- ribbon actions
- command palette entries
- side panel lineage inspector
- vault sync controls
- status health widget

### Success criteria
A user can navigate and replay memory entirely from Obsidian UI.

---

## Phase 7 — Bi-directional intelligence sync (Week 8+)
### What we are doing
Support **human-authored note edits flowing back into Memex**.

Examples:
- user edits summary note
- adds manual rationale
- creates new wikilink bridge
- writes postmortem insights

These become graph updates.

### Why we are doing it
This upgrades the system from projection-only to **co-evolving human + AI memory**.

This is the long-term strategic differentiator.

### Deliverables
- markdown diff parser
- frontmatter stable IDs
- edit reconciliation engine
- conflict resolution rules
- manual edge promotion workflow

### Success criteria
Human insight added in Obsidian improves future Memex retrieval quality.

---

## Engineering principles across all phases
### 1) Never make markdown the truth layer
Always preserve:
- lineage DAG
- branch graph
- state diffs
- confidence metadata
inside Memex.

### 2) Stable identifiers everywhere
Every note should map to:
- thread_id
- snapshot_id
- replay_id
- bridge edge ids

### 3) Incremental sync only
Vault sync must avoid rewriting unchanged files.

### 4) Human-first readability
Generated notes should feel handwritten, not machine dumped.

---

## Final rollout strategy
### MVP ship target
Ship Phases **0–3** first.

This already gives:
- native vault browsing
- live threads
- timeline evolution
- snapshot history

### Growth milestone
Phases **4–6** create the moat:
- replay
- canvas DAGs
- plugin UX

### Strategic moat
Phase **7** creates:
> collaborative human + AI temporal memory

That is extremely difficult for competitors to replicate.

