# Memex Vision — Graph UI Design Spec

**Date:** 2026-04-13  
**Status:** Approved  
**Replaces:** `memex/ui` (to be deleted)

---

## Overview

`memex/vision` is an Obsidian-style visual memory intelligence UI for the Memex backend. It renders memories as a force-directed graph where nodes are memories and edges represent relationships between them. The goal is to make AI memory understandable visually — showing clusters, connections, and patterns across sessions.

---

## App Location

New Vite+React app at `memex/vision/`. The existing `memex/ui/` is removed.

---

## Tech Stack

- React + Vite + TypeScript
- React Flow (`@xyflow/react`) + `d3-force` for force-directed layout
- Tailwind CSS
- shadcn/ui (sidebar items, toggles, badges, tooltips)
- Framer Motion (panel animations, edge fade-ins)
- Backend: existing Memex Go API at `http://localhost:8765`

---

## Layout

Three-column shell:

```
┌──────────────────────────────────────────────────────────────┐
│  [vision]                                    [search........] │  topbar
├─────────────┬────────────────────────────┬───────────────────┤
│  PROJECTS   │                            │  NODE INSPECTOR   │
│  memex      │      React Flow canvas     │  (selected node)  │
│  system-..  │      force-directed        │                   │
│  caveman    │      pan + zoom            │                   │
│             │                            │                   │
│  FILTERS    │                            │                   │
│  memory_type│                            │                   │
│  topic      │                            │                   │
│             │                            │                   │
│  EDGES      │                            │                   │
│  ✓ topic    │                            │                   │
│  ○ KG facts │                            │                   │
│  ○ similar  │                            │                   │
└─────────────┴────────────────────────────┴───────────────────┘
```

- **Topbar**: app name `vision`, global search that filters visible nodes
- **Left sidebar** (240px): project list, memory_type/topic filters, edge layer toggles
- **Center canvas**: React Flow force-directed graph, dark `#0d0d0d` background
- **Right panel** (320px): slides in when a node is selected, collapses when nothing selected

---

## Nodes

Phase 1: Memory nodes only. Other node types (CHECKPOINT, TRACE, SUMMARY, PROJECT) added in later phases.

**Shape:** Circle  
**Size:** Scales with `importance` score (0.0–1.0). Min radius 6px, max 20px.  
**Color by `memory_type`:**

| memory_type | color |
|---|---|
| decision | `#6366f1` indigo |
| preference | `#8b5cf6` violet |
| event | `#3b82f6` blue |
| discovery | `#10b981` emerald |
| advice | `#f59e0b` amber |
| problem | `#ef4444` red |
| context | `#6b7280` gray |
| procedure | `#06b6d4` cyan |
| rationale | `#ec4899` pink |

**Label:** Truncated memory text (~40 chars), shown on hover via tooltip.  
**Selected state:** White ring glow. Unrelated nodes dim to 20% opacity.

---

## Edges

Three lazy layers. `topic/tags` is on by default, others off.

| Layer | Default | Computation | Visual |
|---|---|---|---|
| topic/tags | ON | client-side: shared topic or any tag overlap | thin white line, 15% opacity |
| KG facts | OFF | `GET /facts?subject=` per node, fetched on toggle | colored by predicate, labeled |
| semantic | OFF | `GET /memories/similar?text=` per node (passes memory text), fetched on toggle | dashed line, opacity weighted by score |

KG and semantic fetch results cached in `Map<nodeId, FlowEdge[]>` — toggling off and back on does not refetch.

Edges animate in with a fade when a layer is toggled on.

---

## Data Flow

**Boot sequence:**
1. `GET /trace/projects` → populate project list
2. User selects project → `GET /memories?project=X&limit=200`
3. Client derives topic/tag edges immediately, renders graph
4. Edge layer toggles trigger lazy per-node fetches

**State (React context):**

```ts
AppState {
  projects: string[]
  selectedProject: string | null
  memories: Memory[]
  nodes: FlowNode[]
  edges: FlowEdge[]
  activeEdgeLayers: Set<'topic' | 'kg' | 'similar'>
  edgeCache: Map<string, FlowEdge[]>
  selectedNodeId: string | null
  searchQuery: string
  filters: { memoryType: string | null, topic: string | null }
}
```

**Derivation pipeline** (reactive on memories or activeEdgeLayers change):
```
memories → buildNodes() → FlowNode[]
memories + activeEdgeLayers + edgeCache → buildEdges() → FlowEdge[]
```

---

## Component Structure

```
memex/vision/src/
├── api.ts                    typed fetch wrappers for all backend calls
├── store.tsx                 AppContext + useAppStore hook
├── App.tsx                   root layout shell
├── components/
│   ├── Sidebar.tsx           project list + filters + edge toggles
│   ├── GraphCanvas.tsx       React Flow wrapper, node/edge rendering
│   ├── MemoryNode.tsx        custom React Flow node component
│   ├── NodeInspector.tsx     right panel: memory metadata + full text
│   └── SearchBar.tsx         topbar search, filters visible nodes
└── lib/
    ├── buildNodes.ts         Memory[] → FlowNode[]
    ├── buildEdges.ts         Memory[] + active layers → FlowEdge[]
    └── layout.ts             force simulation config
```

---

## Animations

- Sidebar panel: Framer Motion slide-in from left
- Node inspector: Framer Motion slide-in from right
- Edge layer toggle: edges fade in over 300ms
- Graph nodes: React Flow handles position transitions

---

## API Endpoints Used (Phase 1)

| Endpoint | Purpose |
|---|---|
| `GET /trace/projects` | project list |
| `GET /memories?project=&limit=200` | all memories for selected project |
| `GET /memories?project=&memory_type=&topic=` | filtered memories |
| `GET /memories/similar?text=&project=` | semantic edge layer (passes memory text) |
| `GET /facts?subject=` | KG fact edge layer |

---

## Out of Scope (Phase 1)

- CHECKPOINT, TRACE, SUMMARY, PROJECT node types
- Taxonomy explorer
- Retrieval inspector
- Session replay / time travel
- Supersedes chains
- Confidence scores
