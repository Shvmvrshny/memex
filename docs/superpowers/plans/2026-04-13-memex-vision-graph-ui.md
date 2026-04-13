# Memex Vision Graph UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `memex/vision` — an Obsidian-style force-directed graph UI over the Memex backend that shows memory nodes, colored by type, connected by topic/tag/KG/semantic edge layers.

**Architecture:** Fresh Vite+React+TypeScript app at `memex/vision/`. Memories fetched from the existing Go backend (localhost:8765 via Vite proxy). React Flow renders the graph canvas; d3-force computes initial node positions. Three edge layers (topic/tags always-on, KG facts + semantic similarity lazy-loaded and cached per project).

**Tech Stack:** React 19, Vite, TypeScript, @xyflow/react, d3-force, Tailwind CSS v4, shadcn/ui, Framer Motion

---

## File Map

```
memex/vision/
├── index.html
├── vite.config.ts           proxy /memories /trace /facts → localhost:8765
├── tailwind.config.ts
├── components.json          shadcn config
├── src/
│   ├── main.tsx
│   ├── App.tsx              root shell: topbar + 3-column layout
│   ├── api.ts               typed fetch wrappers (all backend calls)
│   ├── store.tsx            AppContext + useReducer state + dispatch
│   ├── components/
│   │   ├── GraphCanvas.tsx  React Flow wrapper + d3-force layout
│   │   ├── MemoryNode.tsx   custom React Flow node (circle, color, glow)
│   │   ├── NodeInspector.tsx  right panel: memory metadata + text
│   │   ├── SearchBar.tsx    topbar search input
│   │   └── Sidebar.tsx      project list + filters + edge toggles
│   └── lib/
│       ├── colors.ts        MEMORY_TYPE_COLORS constant
│       ├── buildNodes.ts    Memory[] → FlowNode[]
│       ├── buildNodes.test.ts
│       ├── buildEdges.ts    Memory[] + layers + facts → FlowEdge[]
│       ├── buildEdges.test.ts
│       └── layout.ts        d3-force simulation: nodes + edges → positioned nodes
```

---

## Task 1: Scaffold `memex/vision`

**Files:**
- Create: `memex/vision/` (entire project)

- [ ] **Step 1: Create the Vite app**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
npm create vite@latest vision -- --template react-ts
cd vision
```

- [ ] **Step 2: Install all dependencies**

```bash
npm install @xyflow/react d3-force framer-motion
npm install --save-dev @types/d3-force vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/jest-dom
npm install tailwindcss @tailwindcss/vite
```

- [ ] **Step 3: Replace `vite.config.ts`**

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/memories': 'http://localhost:8765',
      '/trace': 'http://localhost:8765',
      '/facts': 'http://localhost:8765',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
  },
})
```

- [ ] **Step 4: Replace `src/index.css` with Tailwind + dark base**

```css
@import "tailwindcss";

:root {
  color-scheme: dark;
}

body {
  background: #0d0d0d;
  color: #e4e4e7;
  font-family: ui-sans-serif, system-ui, sans-serif;
  margin: 0;
}

* {
  box-sizing: border-box;
}
```

- [ ] **Step 5: Stub `src/main.tsx`**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

- [ ] **Step 6: Stub `src/App.tsx` so the project builds**

```tsx
export default function App() {
  return <div className="h-screen bg-[#0d0d0d] text-zinc-100">vision</div>
}
```

- [ ] **Step 7: Verify the project builds and runs**

```bash
npm run dev
```

Expected: Vite dev server starts, browser shows "vision" on dark background.

- [ ] **Step 8: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/
git commit -m "feat: scaffold memex/vision vite app"
```

---

## Task 2: Remove `memex/ui`

**Files:**
- Delete: `memex/ui/`

- [ ] **Step 1: Delete the old UI**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
rm -rf ui/
```

- [ ] **Step 2: Commit**

```bash
git add -A
git commit -m "chore: remove memex/ui (replaced by vision)"
```

---

## Task 3: `src/api.ts` — Typed fetch wrappers

**Files:**
- Create: `memex/vision/src/api.ts`

- [ ] **Step 1: Write `api.ts`**

```ts
// All paths are relative — Vite proxies them to http://localhost:8765

export interface Memory {
  id: string
  text: string
  project: string
  topic: string
  memory_type: string
  source: string
  timestamp: string
  importance: number
  tags: string[]
  last_accessed: string
  score?: number
}

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

export async function fetchProjects(): Promise<string[]> {
  const res = await fetch('/trace/projects')
  if (!res.ok) throw new Error('Failed to fetch projects')
  return res.json()
}

export async function fetchMemories(
  project: string,
  memoryType?: string | null,
  topic?: string | null,
): Promise<Memory[]> {
  const params = new URLSearchParams({ project, limit: '200' })
  if (memoryType) params.set('memory_type', memoryType)
  if (topic) params.set('topic', topic)
  const res = await fetch(`/memories?${params}`)
  if (!res.ok) throw new Error('Failed to fetch memories')
  const data = await res.json()
  return data.memories ?? []
}

export async function fetchSimilarMemories(
  text: string,
  project: string,
): Promise<Memory[]> {
  const params = new URLSearchParams({ text, project, limit: '8' })
  const res = await fetch(`/memories/similar?${params}`)
  if (!res.ok) throw new Error('Failed to fetch similar memories')
  const data = await res.json()
  return data.memories ?? []
}

export async function fetchFacts(subject: string): Promise<Fact[]> {
  const params = new URLSearchParams({ subject })
  const res = await fetch(`/facts?${params}`)
  if (!res.ok) throw new Error('Failed to fetch facts')
  const data = await res.json()
  // /facts returns an array directly
  return Array.isArray(data) ? data : []
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/api.ts
git commit -m "feat(vision): add typed API layer"
```

---

## Task 4: `src/lib/colors.ts`

**Files:**
- Create: `memex/vision/src/lib/colors.ts`

- [ ] **Step 1: Write `colors.ts`**

```ts
export const MEMORY_TYPE_COLORS: Record<string, string> = {
  decision:   '#6366f1',
  preference: '#8b5cf6',
  event:      '#3b82f6',
  discovery:  '#10b981',
  advice:     '#f59e0b',
  problem:    '#ef4444',
  context:    '#6b7280',
  procedure:  '#06b6d4',
  rationale:  '#ec4899',
}

export const DEFAULT_NODE_COLOR = '#6b7280'

/** Map importance [0, 1] → radius [6, 20] px */
export function importanceToRadius(importance: number): number {
  const clamped = Math.max(0, Math.min(1, importance))
  return 6 + clamped * 14
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/lib/colors.ts
git commit -m "feat(vision): add memory type colors + radius helper"
```

---

## Task 5: `src/lib/buildNodes.ts` — TDD

**Files:**
- Create: `memex/vision/src/lib/buildNodes.ts`
- Create: `memex/vision/src/lib/buildNodes.test.ts`

- [ ] **Step 1: Write the failing tests**

```ts
// src/lib/buildNodes.test.ts
import { describe, it, expect } from 'vitest'
import { buildNodes } from './buildNodes'
import type { Memory } from '../api'

const base: Memory = {
  id: 'x',
  text: 'some memory text',
  project: 'myproject',
  topic: 'testing',
  memory_type: 'preference',
  source: 'claude',
  timestamp: '2024-01-01T00:00:00Z',
  importance: 0.5,
  tags: [],
  last_accessed: '2024-01-01T00:00:00Z',
}

describe('buildNodes', () => {
  it('maps each memory to a FlowNode with matching id', () => {
    const nodes = buildNodes([base])
    expect(nodes).toHaveLength(1)
    expect(nodes[0].id).toBe('x')
  })

  it('sets node type to "memory"', () => {
    const nodes = buildNodes([base])
    expect(nodes[0].type).toBe('memory')
  })

  it('attaches the memory object to node data', () => {
    const nodes = buildNodes([base])
    expect(nodes[0].data.memory).toEqual(base)
  })

  it('computes radius from importance: higher importance = larger radius', () => {
    const big = buildNodes([{ ...base, id: 'a', importance: 1.0 }])
    const small = buildNodes([{ ...base, id: 'b', importance: 0.0 }])
    expect(big[0].data.radius).toBeGreaterThan(small[0].data.radius)
  })

  it('sets radius min 6 for importance 0', () => {
    const nodes = buildNodes([{ ...base, importance: 0 }])
    expect(nodes[0].data.radius).toBe(6)
  })

  it('sets radius max 20 for importance 1', () => {
    const nodes = buildNodes([{ ...base, importance: 1 }])
    expect(nodes[0].data.radius).toBe(20)
  })

  it('sets a default position of {x:0, y:0}', () => {
    const nodes = buildNodes([base])
    expect(nodes[0].position).toEqual({ x: 0, y: 0 })
  })

  it('handles empty array', () => {
    expect(buildNodes([])).toEqual([])
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd /Users/shivamvarshney/Documents/projects/memex/vision
npx vitest run src/lib/buildNodes.test.ts
```

Expected: FAIL — `buildNodes` not found.

- [ ] **Step 3: Implement `buildNodes.ts`**

```ts
// src/lib/buildNodes.ts
import type { Node as FlowNode } from '@xyflow/react'
import type { Memory } from '../api'
import { importanceToRadius } from './colors'

export interface MemoryNodeData {
  memory: Memory
  radius: number
  [key: string]: unknown
}

export function buildNodes(memories: Memory[]): FlowNode<MemoryNodeData>[] {
  return memories.map((m) => ({
    id: m.id,
    type: 'memory',
    position: { x: 0, y: 0 },
    data: {
      memory: m,
      radius: importanceToRadius(m.importance),
    },
  }))
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
npx vitest run src/lib/buildNodes.test.ts
```

Expected: All 8 tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/lib/buildNodes.ts vision/src/lib/buildNodes.test.ts
git commit -m "feat(vision): add buildNodes with tests"
```

---

## Task 6: `src/lib/buildEdges.ts` — TDD (topic/tag + KG + semantic layers)

**Files:**
- Create: `memex/vision/src/lib/buildEdges.ts`
- Create: `memex/vision/src/lib/buildEdges.test.ts`

- [ ] **Step 1: Write failing tests**

```ts
// src/lib/buildEdges.test.ts
import { describe, it, expect } from 'vitest'
import { buildTopicEdges, buildKGEdges, buildSemanticEdges } from './buildEdges'
import type { Memory, Fact } from '../api'
import type { Edge as FlowEdge } from '@xyflow/react'

const mem = (overrides: Partial<Memory> & { id: string }): Memory => ({
  text: 'text',
  project: 'p',
  topic: '',
  memory_type: 'context',
  source: 'claude',
  timestamp: '2024-01-01T00:00:00Z',
  importance: 0.5,
  tags: [],
  last_accessed: '2024-01-01T00:00:00Z',
  ...overrides,
})

describe('buildTopicEdges', () => {
  it('connects memories sharing the same topic', () => {
    const edges = buildTopicEdges([
      mem({ id: 'a', topic: 'testing' }),
      mem({ id: 'b', topic: 'testing' }),
      mem({ id: 'c', topic: 'other' }),
    ])
    expect(edges).toHaveLength(1)
    expect(edges[0].source).toBe('a')
    expect(edges[0].target).toBe('b')
  })

  it('connects memories sharing any tag', () => {
    const edges = buildTopicEdges([
      mem({ id: 'a', tags: ['go', 'tdd'] }),
      mem({ id: 'b', tags: ['go'] }),
      mem({ id: 'c', tags: ['python'] }),
    ])
    expect(edges).toHaveLength(1)
    expect(edges[0].source).toBe('a')
    expect(edges[0].target).toBe('b')
  })

  it('does not create duplicate edges', () => {
    const edges = buildTopicEdges([
      mem({ id: 'a', topic: 'x', tags: ['t'] }),
      mem({ id: 'b', topic: 'x', tags: ['t'] }),
    ])
    expect(edges).toHaveLength(1)
  })

  it('does not create self-edges', () => {
    const edges = buildTopicEdges([mem({ id: 'a', topic: 'x' })])
    expect(edges).toHaveLength(0)
  })

  it('returns empty for memories with no overlap', () => {
    const edges = buildTopicEdges([
      mem({ id: 'a', topic: 'foo' }),
      mem({ id: 'b', topic: 'bar' }),
    ])
    expect(edges).toHaveLength(0)
  })

  it('handles empty input', () => {
    expect(buildTopicEdges([])).toEqual([])
  })
})

describe('buildKGEdges', () => {
  it('connects memories whose topics match a fact subject→object', () => {
    const memories = [
      mem({ id: 'a', topic: 'memex' }),
      mem({ id: 'b', topic: 'qdrant' }),
    ]
    const facts: Fact[] = [{
      id: 'f1', subject: 'memex', predicate: 'uses', object: 'qdrant',
      created_at: '2024-01-01',
    }]
    const edges = buildKGEdges(memories, facts)
    expect(edges).toHaveLength(1)
    expect(edges[0].source).toBe('a')
    expect(edges[0].target).toBe('b')
    expect(edges[0].label).toBe('uses')
  })

  it('returns empty when no topic matches any fact', () => {
    const memories = [mem({ id: 'a', topic: 'foo' }), mem({ id: 'b', topic: 'bar' })]
    const facts: Fact[] = [{ id: 'f', subject: 'x', predicate: 'y', object: 'z', created_at: '2024-01-01' }]
    expect(buildKGEdges(memories, facts)).toHaveLength(0)
  })

  it('handles empty facts array', () => {
    expect(buildKGEdges([mem({ id: 'a', topic: 'x' })], [])).toHaveLength(0)
  })
})

describe('buildSemanticEdges', () => {
  it('creates an edge from source to each similar memory', () => {
    const source = mem({ id: 'a' })
    const similar = [mem({ id: 'b' }), mem({ id: 'c' })]
    const edges = buildSemanticEdges(source, similar)
    expect(edges).toHaveLength(2)
    expect(edges.every(e => e.source === 'a')).toBe(true)
  })

  it('excludes the source memory from similar results', () => {
    const source = mem({ id: 'a' })
    const similar = [mem({ id: 'a' }), mem({ id: 'b' })]
    const edges = buildSemanticEdges(source, similar)
    expect(edges).toHaveLength(1)
    expect(edges[0].target).toBe('b')
  })

  it('handles empty similar array', () => {
    expect(buildSemanticEdges(mem({ id: 'a' }), [])).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd /Users/shivamvarshney/Documents/projects/memex/vision
npx vitest run src/lib/buildEdges.test.ts
```

Expected: FAIL — functions not found.

- [ ] **Step 3: Implement `buildEdges.ts`**

```ts
// src/lib/buildEdges.ts
import type { Edge as FlowEdge } from '@xyflow/react'
import type { Memory, Fact } from '../api'

export function buildTopicEdges(memories: Memory[]): FlowEdge[] {
  const edges: FlowEdge[] = []
  const seen = new Set<string>()

  for (let i = 0; i < memories.length; i++) {
    for (let j = i + 1; j < memories.length; j++) {
      const a = memories[i]
      const b = memories[j]
      const shareTopic = Boolean(a.topic && b.topic && a.topic === b.topic)
      const shareTag = a.tags.some((t) => b.tags.includes(t))
      if (!shareTopic && !shareTag) continue

      const key = `${a.id}-${b.id}`
      if (seen.has(key)) continue
      seen.add(key)

      edges.push({
        id: `topic-${a.id}-${b.id}`,
        source: a.id,
        target: b.id,
        style: { stroke: 'rgba(255,255,255,0.15)', strokeWidth: 1 },
      })
    }
  }
  return edges
}

export function buildKGEdges(memories: Memory[], facts: Fact[]): FlowEdge[] {
  const byTopic = new Map<string, Memory[]>()
  for (const m of memories) {
    if (!m.topic) continue
    const arr = byTopic.get(m.topic) ?? []
    arr.push(m)
    byTopic.set(m.topic, arr)
  }

  const edges: FlowEdge[] = []
  for (const fact of facts) {
    const sources = byTopic.get(fact.subject) ?? []
    const targets = byTopic.get(fact.object) ?? []
    for (const s of sources) {
      for (const t of targets) {
        if (s.id === t.id) continue
        edges.push({
          id: `kg-${fact.id}-${s.id}-${t.id}`,
          source: s.id,
          target: t.id,
          label: fact.predicate,
          style: { stroke: '#6366f1', strokeWidth: 1 },
        })
      }
    }
  }
  return edges
}

export function buildSemanticEdges(
  sourceMemory: Memory,
  similarMemories: Memory[],
): FlowEdge[] {
  return similarMemories
    .filter((m) => m.id !== sourceMemory.id)
    .map((m) => ({
      id: `sim-${sourceMemory.id}-${m.id}`,
      source: sourceMemory.id,
      target: m.id,
      style: {
        stroke: 'rgba(16,185,129,0.5)',
        strokeWidth: 1,
        strokeDasharray: '4 2',
      },
    }))
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
npx vitest run src/lib/buildEdges.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/lib/buildEdges.ts vision/src/lib/buildEdges.test.ts
git commit -m "feat(vision): add buildEdges (topic, KG, semantic) with tests"
```

---

## Task 7: `src/lib/layout.ts` — d3-force simulation

**Files:**
- Create: `memex/vision/src/lib/layout.ts`

- [ ] **Step 1: Write `layout.ts`**

```ts
import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  type SimulationNodeDatum,
} from 'd3-force'
import type { Node as FlowNode, Edge as FlowEdge } from '@xyflow/react'

interface SimNode extends SimulationNodeDatum {
  id: string
}

/**
 * Runs a d3-force simulation synchronously and returns nodes with positions set.
 * Call this after fetching memories; React Flow then renders from those positions.
 */
export function applyForceLayout(
  nodes: FlowNode[],
  edges: FlowEdge[],
): FlowNode[] {
  if (nodes.length === 0) return nodes

  const simNodes: SimNode[] = nodes.map((n) => ({
    id: n.id,
    x: (Math.random() - 0.5) * 800,
    y: (Math.random() - 0.5) * 600,
  }))

  const nodeIds = new Set(nodes.map((n) => n.id))
  const simLinks = edges
    .filter((e) => nodeIds.has(e.source as string) && nodeIds.has(e.target as string))
    .map((e) => ({ source: e.source as string, target: e.target as string }))

  const sim = forceSimulation(simNodes)
    .force(
      'link',
      forceLink(simLinks)
        .id((d) => (d as SimNode).id)
        .distance(100)
        .strength(0.4),
    )
    .force('charge', forceManyBody().strength(-250))
    .force('center', forceCenter(0, 0))
    .stop()

  sim.tick(300)

  return nodes.map((n, i) => ({
    ...n,
    position: {
      x: simNodes[i].x ?? 0,
      y: simNodes[i].y ?? 0,
    },
  }))
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/lib/layout.ts
git commit -m "feat(vision): add d3-force layout helper"
```

---

## Task 8: `src/store.tsx` — App state

**Files:**
- Create: `memex/vision/src/store.tsx`

- [ ] **Step 1: Write `store.tsx`**

```tsx
import {
  createContext,
  useContext,
  useReducer,
  useEffect,
  type ReactNode,
  type Dispatch,
} from 'react'
import type { Node as FlowNode, Edge as FlowEdge } from '@xyflow/react'
import type { Memory } from './api'
import { fetchProjects, fetchMemories, fetchSimilarMemories, fetchFacts } from './api'
import { buildNodes } from './lib/buildNodes'
import { buildTopicEdges, buildKGEdges, buildSemanticEdges } from './lib/buildEdges'
import { applyForceLayout } from './lib/layout'

export type EdgeLayer = 'topic' | 'kg' | 'similar'

interface AppState {
  projects: string[]
  selectedProject: string | null
  memories: Memory[]
  nodes: FlowNode[]
  edges: FlowEdge[]
  activeEdgeLayers: Set<EdgeLayer>
  /** Cached edges per layer (cleared when project changes) */
  edgeCache: Map<EdgeLayer, FlowEdge[]>
  selectedNodeId: string | null
  searchQuery: string
  filters: { memoryType: string | null; topic: string | null }
  loading: boolean
  error: string | null
}

type Action =
  | { type: 'SET_PROJECTS'; projects: string[] }
  | { type: 'SELECT_PROJECT'; project: string }
  | { type: 'SET_MEMORIES'; memories: Memory[] }
  | { type: 'SELECT_NODE'; id: string | null }
  | { type: 'SET_SEARCH'; query: string }
  | { type: 'SET_FILTER'; key: 'memoryType' | 'topic'; value: string | null }
  | { type: 'TOGGLE_EDGE_LAYER'; layer: EdgeLayer }
  | { type: 'CACHE_LAYER_EDGES'; layer: EdgeLayer; edges: FlowEdge[] }
  | { type: 'SET_LOADING'; loading: boolean }
  | { type: 'SET_ERROR'; error: string | null }

function deriveEdges(
  memories: Memory[],
  activeLayers: Set<EdgeLayer>,
  cache: Map<EdgeLayer, FlowEdge[]>,
): FlowEdge[] {
  const all: FlowEdge[] = []
  if (activeLayers.has('topic')) all.push(...buildTopicEdges(memories))
  if (activeLayers.has('kg')) all.push(...(cache.get('kg') ?? []))
  if (activeLayers.has('similar')) all.push(...(cache.get('similar') ?? []))
  return all
}

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'SET_PROJECTS':
      return { ...state, projects: action.projects }

    case 'SELECT_PROJECT': {
      return {
        ...state,
        selectedProject: action.project,
        memories: [],
        nodes: [],
        edges: [],
        edgeCache: new Map(),
        selectedNodeId: null,
        searchQuery: '',
        filters: { memoryType: null, topic: null },
      }
    }

    case 'SET_MEMORIES': {
      const rawNodes = buildNodes(action.memories)
      const topicEdges = buildTopicEdges(action.memories)
      const newCache = new Map(state.edgeCache)
      // topic edges don't need caching — recomputed, but we keep KG/similar cache if any
      const edges = deriveEdges(action.memories, state.activeEdgeLayers, newCache)
      const positioned = applyForceLayout(rawNodes, topicEdges)
      return {
        ...state,
        memories: action.memories,
        nodes: positioned,
        edges,
      }
    }

    case 'SELECT_NODE':
      return { ...state, selectedNodeId: action.id }

    case 'SET_SEARCH':
      return { ...state, searchQuery: action.query }

    case 'SET_FILTER':
      return {
        ...state,
        filters: { ...state.filters, [action.key]: action.value },
      }

    case 'TOGGLE_EDGE_LAYER': {
      const next = new Set(state.activeEdgeLayers)
      next.has(action.layer) ? next.delete(action.layer) : next.add(action.layer)
      const edges = deriveEdges(state.memories, next, state.edgeCache)
      return { ...state, activeEdgeLayers: next, edges }
    }

    case 'CACHE_LAYER_EDGES': {
      const newCache = new Map(state.edgeCache)
      newCache.set(action.layer, action.edges)
      const edges = deriveEdges(state.memories, state.activeEdgeLayers, newCache)
      return { ...state, edgeCache: newCache, edges }
    }

    case 'SET_LOADING':
      return { ...state, loading: action.loading }

    case 'SET_ERROR':
      return { ...state, error: action.error }

    default:
      return state
  }
}

const initialState: AppState = {
  projects: [],
  selectedProject: null,
  memories: [],
  nodes: [],
  edges: [],
  activeEdgeLayers: new Set(['topic']),
  edgeCache: new Map(),
  selectedNodeId: null,
  searchQuery: '',
  filters: { memoryType: null, topic: null },
  loading: false,
  error: null,
}

const AppContext = createContext<{
  state: AppState
  dispatch: Dispatch<Action>
} | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState)

  // Load projects on mount
  useEffect(() => {
    fetchProjects()
      .then((projects) => dispatch({ type: 'SET_PROJECTS', projects }))
      .catch((e) => dispatch({ type: 'SET_ERROR', error: String(e) }))
  }, [])

  // Load memories when project or filters change
  useEffect(() => {
    if (!state.selectedProject) return
    dispatch({ type: 'SET_LOADING', loading: true })
    fetchMemories(state.selectedProject, state.filters.memoryType, state.filters.topic)
      .then((memories) => dispatch({ type: 'SET_MEMORIES', memories }))
      .catch((e) => dispatch({ type: 'SET_ERROR', error: String(e) }))
      .finally(() => dispatch({ type: 'SET_LOADING', loading: false }))
  }, [state.selectedProject, state.filters.memoryType, state.filters.topic])

  // Lazy-load KG edges when that layer is toggled on and not yet cached
  useEffect(() => {
    if (
      !state.activeEdgeLayers.has('kg') ||
      state.edgeCache.has('kg') ||
      state.memories.length === 0
    ) return

    const uniqueTopics = [...new Set(state.memories.map((m) => m.topic).filter(Boolean))]
    Promise.all(uniqueTopics.map((t) => fetchFacts(t)))
      .then((results) => {
        const allFacts = results.flat()
        const kgEdges = buildKGEdges(state.memories, allFacts)
        dispatch({ type: 'CACHE_LAYER_EDGES', layer: 'kg', edges: kgEdges })
      })
      .catch((e) => dispatch({ type: 'SET_ERROR', error: String(e) }))
  }, [state.activeEdgeLayers, state.memories, state.edgeCache])

  // Lazy-load semantic edges when that layer is toggled on and not yet cached
  useEffect(() => {
    if (
      !state.activeEdgeLayers.has('similar') ||
      state.edgeCache.has('similar') ||
      state.memories.length === 0 ||
      !state.selectedProject
    ) return

    Promise.all(
      state.memories.map((m) =>
        fetchSimilarMemories(m.text, state.selectedProject!).then((similar) =>
          buildSemanticEdges(m, similar),
        ),
      ),
    )
      .then((edgeArrays) => {
        const allEdges = edgeArrays.flat()
        // Deduplicate: keep only one edge per pair (a-b or b-a)
        const seen = new Set<string>()
        const unique = allEdges.filter((e) => {
          const key = [e.source, e.target].sort().join('-')
          if (seen.has(key)) return false
          seen.add(key)
          return true
        })
        dispatch({ type: 'CACHE_LAYER_EDGES', layer: 'similar', edges: unique })
      })
      .catch((e) => dispatch({ type: 'SET_ERROR', error: String(e) }))
  }, [state.activeEdgeLayers, state.memories, state.edgeCache, state.selectedProject])

  return (
    <AppContext.Provider value={{ state, dispatch }}>
      {children}
    </AppContext.Provider>
  )
}

export function useAppStore() {
  const ctx = useContext(AppContext)
  if (!ctx) throw new Error('useAppStore must be used within AppProvider')
  return ctx
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/store.tsx
git commit -m "feat(vision): add AppContext store with lazy edge layer loading"
```

---

## Task 9: `src/components/MemoryNode.tsx` — Custom React Flow node

**Files:**
- Create: `memex/vision/src/components/MemoryNode.tsx`

- [ ] **Step 1: Write `MemoryNode.tsx`**

```tsx
import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { MemoryNodeData } from '../lib/buildNodes'
import { MEMORY_TYPE_COLORS, DEFAULT_NODE_COLOR } from '../lib/colors'

export const MemoryNode = memo(function MemoryNode({
  data,
  selected,
}: NodeProps<MemoryNodeData & { [key: string]: unknown }>) {
  const r = data.radius as number
  const memory = data.memory as MemoryNodeData['memory']
  const color = MEMORY_TYPE_COLORS[memory.memory_type] ?? DEFAULT_NODE_COLOR
  const size = r * 2

  return (
    <>
      {/* Invisible handles so React Flow doesn't render its default UI */}
      <Handle type="target" position={Position.Top} style={{ opacity: 0 }} />
      <div
        title={memory.text.slice(0, 80)}
        style={{
          width: size,
          height: size,
          borderRadius: '50%',
          backgroundColor: color,
          boxShadow: selected
            ? `0 0 0 2px #fff, 0 0 12px ${color}`
            : `0 0 4px ${color}44`,
          cursor: 'pointer',
          transition: 'box-shadow 0.15s ease',
        }}
      />
      <Handle type="source" position={Position.Bottom} style={{ opacity: 0 }} />
    </>
  )
})
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/components/MemoryNode.tsx
git commit -m "feat(vision): add custom MemoryNode component"
```

---

## Task 10: `src/components/GraphCanvas.tsx` — React Flow canvas

**Files:**
- Create: `memex/vision/src/components/GraphCanvas.tsx`

- [ ] **Step 1: Write `GraphCanvas.tsx`**

```tsx
import { useCallback, useMemo } from 'react'
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  Controls,
  type NodeMouseHandler,
  type Node as FlowNode,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useAppStore } from '../store'
import { MemoryNode } from './MemoryNode'
import type { MemoryNodeData } from '../lib/buildNodes'

const nodeTypes = { memory: MemoryNode }

export function GraphCanvas() {
  const { state, dispatch } = useAppStore()
  const { nodes, edges, selectedNodeId, searchQuery } = state

  // Filter nodes by search query; dim unrelated nodes when one is selected
  const visibleNodes = useMemo(() => {
    const q = searchQuery.toLowerCase()
    return nodes.map((n) => {
      const mem = (n.data as MemoryNodeData).memory
      const matchesSearch = !q || mem.text.toLowerCase().includes(q) || mem.topic.toLowerCase().includes(q)
      const isSelected = n.id === selectedNodeId
      const isNeighbor = selectedNodeId
        ? edges.some(
            (e) =>
              (e.source === selectedNodeId && e.target === n.id) ||
              (e.target === selectedNodeId && e.source === n.id),
          )
        : false

      const dimmed = selectedNodeId ? !isSelected && !isNeighbor : false
      const hidden = !matchesSearch

      return {
        ...n,
        hidden,
        style: {
          opacity: hidden ? 0 : dimmed ? 0.15 : 1,
          transition: 'opacity 0.2s ease',
        },
      }
    })
  }, [nodes, edges, selectedNodeId, searchQuery])

  const visibleEdges = useMemo(() => {
    if (!searchQuery) return edges
    const visibleIds = new Set(visibleNodes.filter((n) => !n.hidden).map((n) => n.id))
    return edges.filter((e) => visibleIds.has(e.source as string) && visibleIds.has(e.target as string))
  }, [edges, visibleNodes, searchQuery])

  const onNodeClick: NodeMouseHandler = useCallback(
    (_event, node: FlowNode) => {
      dispatch({
        type: 'SELECT_NODE',
        id: node.id === selectedNodeId ? null : node.id,
      })
    },
    [dispatch, selectedNodeId],
  )

  const onPaneClick = useCallback(() => {
    dispatch({ type: 'SELECT_NODE', id: null })
  }, [dispatch])

  return (
    <div className="w-full h-full">
      <ReactFlow
        nodes={visibleNodes}
        edges={visibleEdges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        fitView
        minZoom={0.1}
        maxZoom={4}
        style={{ background: '#0d0d0d' }}
        proOptions={{ hideAttribution: true }}
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={24}
          size={1}
          color="#1f1f1f"
        />
        <Controls
          style={{ background: '#1a1a1a', border: '1px solid #27272a' }}
        />
      </ReactFlow>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/components/GraphCanvas.tsx
git commit -m "feat(vision): add GraphCanvas with node dimming + search filter"
```

---

## Task 11: `src/components/NodeInspector.tsx` — Right metadata panel

**Files:**
- Create: `memex/vision/src/components/NodeInspector.tsx`

- [ ] **Step 1: Write `NodeInspector.tsx`**

```tsx
import { AnimatePresence, motion } from 'framer-motion'
import { useAppStore } from '../store'
import { MEMORY_TYPE_COLORS, DEFAULT_NODE_COLOR } from '../lib/colors'

export function NodeInspector() {
  const { state, dispatch } = useAppStore()
  const { selectedNodeId, memories } = state

  const memory = selectedNodeId
    ? memories.find((m) => m.id === selectedNodeId) ?? null
    : null

  const color = memory
    ? (MEMORY_TYPE_COLORS[memory.memory_type] ?? DEFAULT_NODE_COLOR)
    : DEFAULT_NODE_COLOR

  return (
    <AnimatePresence>
      {memory && (
        <motion.aside
          key="inspector"
          initial={{ x: 320, opacity: 0 }}
          animate={{ x: 0, opacity: 1 }}
          exit={{ x: 320, opacity: 0 }}
          transition={{ type: 'spring', stiffness: 300, damping: 30 }}
          className="w-80 flex-shrink-0 border-l border-zinc-800 bg-[#111] overflow-y-auto flex flex-col"
        >
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-zinc-800">
            <span
              className="text-xs font-semibold uppercase tracking-widest px-2 py-0.5 rounded"
              style={{ background: `${color}22`, color }}
            >
              {memory.memory_type}
            </span>
            <button
              onClick={() => dispatch({ type: 'SELECT_NODE', id: null })}
              className="text-zinc-500 hover:text-zinc-200 text-lg leading-none"
            >
              ×
            </button>
          </div>

          {/* Memory text */}
          <div className="px-4 py-4 text-sm text-zinc-100 leading-relaxed border-b border-zinc-800">
            {memory.text}
          </div>

          {/* Metadata */}
          <div className="px-4 py-4 flex flex-col gap-3 text-xs text-zinc-400">
            {memory.topic && (
              <Row label="topic" value={memory.topic} />
            )}
            {memory.project && (
              <Row label="project" value={memory.project} />
            )}
            <Row label="importance" value={memory.importance.toFixed(2)} />
            <Row
              label="created"
              value={new Date(memory.timestamp).toLocaleDateString()}
            />
            {memory.source && (
              <Row label="source" value={memory.source} />
            )}
            {memory.tags.length > 0 && (
              <div className="flex flex-col gap-1">
                <span className="text-zinc-600">tags</span>
                <div className="flex flex-wrap gap-1">
                  {memory.tags.map((tag) => (
                    <span
                      key={tag}
                      className="px-2 py-0.5 rounded bg-zinc-800 text-zinc-300"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-zinc-600">{label}</span>
      <span className="text-zinc-300">{value}</span>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/components/NodeInspector.tsx
git commit -m "feat(vision): add NodeInspector panel with Framer Motion slide-in"
```

---

## Task 12: `src/components/Sidebar.tsx` — Left panel

**Files:**
- Create: `memex/vision/src/components/Sidebar.tsx`

- [ ] **Step 1: Write `Sidebar.tsx`**

```tsx
import { useAppStore, type EdgeLayer } from '../store'
import { MEMORY_TYPE_COLORS } from '../lib/colors'

const MEMORY_TYPES = [
  'decision', 'preference', 'event', 'discovery',
  'advice', 'problem', 'context', 'procedure', 'rationale',
]

const EDGE_LAYERS: { id: EdgeLayer; label: string }[] = [
  { id: 'topic', label: 'topic / tags' },
  { id: 'kg', label: 'KG facts' },
  { id: 'similar', label: 'semantic' },
]

export function Sidebar() {
  const { state, dispatch } = useAppStore()
  const { projects, selectedProject, filters, activeEdgeLayers, memories, loading } = state

  const uniqueTopics = [...new Set(memories.map((m) => m.topic).filter(Boolean))].sort()

  return (
    <aside className="w-60 flex-shrink-0 border-r border-zinc-800 bg-[#111] overflow-y-auto flex flex-col">
      {/* App title */}
      <div className="px-4 py-4 border-b border-zinc-800">
        <span className="text-base font-semibold tracking-tight text-white">vision</span>
        {loading && (
          <span className="ml-2 text-xs text-zinc-500 animate-pulse">loading…</span>
        )}
      </div>

      {/* Projects */}
      <Section title="Projects">
        {projects.length === 0 ? (
          <p className="text-xs text-zinc-600 px-1">No projects found</p>
        ) : (
          projects.map((p) => (
            <button
              key={p}
              onClick={() => dispatch({ type: 'SELECT_PROJECT', project: p })}
              className={[
                'w-full text-left text-sm px-2 py-1.5 rounded transition-colors',
                selectedProject === p
                  ? 'bg-zinc-700 text-white'
                  : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200',
              ].join(' ')}
            >
              {p}
            </button>
          ))
        )}
      </Section>

      {/* Filter: memory type */}
      {selectedProject && (
        <Section title="Type">
          <button
            onClick={() => dispatch({ type: 'SET_FILTER', key: 'memoryType', value: null })}
            className={filterClass(!filters.memoryType)}
          >
            all
          </button>
          {MEMORY_TYPES.map((t) => (
            <button
              key={t}
              onClick={() =>
                dispatch({
                  type: 'SET_FILTER',
                  key: 'memoryType',
                  value: filters.memoryType === t ? null : t,
                })
              }
              className={filterClass(filters.memoryType === t)}
            >
              <span
                className="inline-block w-2 h-2 rounded-full mr-1.5"
                style={{ background: MEMORY_TYPE_COLORS[t] ?? '#6b7280' }}
              />
              {t}
            </button>
          ))}
        </Section>
      )}

      {/* Filter: topic */}
      {uniqueTopics.length > 0 && (
        <Section title="Topic">
          <button
            onClick={() => dispatch({ type: 'SET_FILTER', key: 'topic', value: null })}
            className={filterClass(!filters.topic)}
          >
            all
          </button>
          {uniqueTopics.map((t) => (
            <button
              key={t}
              onClick={() =>
                dispatch({
                  type: 'SET_FILTER',
                  key: 'topic',
                  value: filters.topic === t ? null : t,
                })
              }
              className={filterClass(filters.topic === t)}
            >
              {t}
            </button>
          ))}
        </Section>
      )}

      {/* Edge layers */}
      {selectedProject && (
        <Section title="Edges">
          {EDGE_LAYERS.map(({ id, label }) => (
            <label
              key={id}
              className="flex items-center gap-2 px-1 py-1 cursor-pointer select-none"
            >
              <input
                type="checkbox"
                checked={activeEdgeLayers.has(id)}
                onChange={() => dispatch({ type: 'TOGGLE_EDGE_LAYER', layer: id })}
                className="accent-indigo-500"
              />
              <span className="text-sm text-zinc-400">{label}</span>
            </label>
          ))}
        </Section>
      )}
    </aside>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="px-3 py-3 border-b border-zinc-800">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-zinc-600 mb-2">
        {title}
      </p>
      <div className="flex flex-col gap-0.5">{children}</div>
    </div>
  )
}

function filterClass(active: boolean) {
  return [
    'w-full text-left text-xs px-2 py-1.5 rounded transition-colors flex items-center',
    active
      ? 'bg-zinc-700 text-white'
      : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200',
  ].join(' ')
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/components/Sidebar.tsx
git commit -m "feat(vision): add Sidebar with project list, filters, edge toggles"
```

---

## Task 13: `src/components/SearchBar.tsx` — Topbar search

**Files:**
- Create: `memex/vision/src/components/SearchBar.tsx`

- [ ] **Step 1: Write `SearchBar.tsx`**

```tsx
import { useAppStore } from '../store'

export function SearchBar() {
  const { state, dispatch } = useAppStore()

  return (
    <input
      type="search"
      value={state.searchQuery}
      onChange={(e) => dispatch({ type: 'SET_SEARCH', query: e.target.value })}
      placeholder="Search memories…"
      className={[
        'w-64 px-3 py-1.5 text-sm rounded-md',
        'bg-zinc-800 border border-zinc-700',
        'text-zinc-200 placeholder-zinc-500',
        'focus:outline-none focus:border-zinc-500',
        'transition-colors',
      ].join(' ')}
    />
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/components/SearchBar.tsx
git commit -m "feat(vision): add SearchBar component"
```

---

## Task 14: Wire `src/App.tsx` — Root shell

**Files:**
- Modify: `memex/vision/src/App.tsx`
- Modify: `memex/vision/src/main.tsx`

- [ ] **Step 1: Update `main.tsx` to wrap with AppProvider**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AppProvider } from './store.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppProvider>
      <App />
    </AppProvider>
  </StrictMode>,
)
```

- [ ] **Step 2: Write full `App.tsx`**

```tsx
import { useAppStore } from './store'
import { Sidebar } from './components/Sidebar'
import { GraphCanvas } from './components/GraphCanvas'
import { NodeInspector } from './components/NodeInspector'
import { SearchBar } from './components/SearchBar'

export default function App() {
  const { state } = useAppStore()

  return (
    <div className="flex flex-col h-screen bg-[#0d0d0d] text-zinc-100 overflow-hidden">
      {/* Topbar */}
      <header className="flex-shrink-0 flex items-center justify-between px-4 py-2 border-b border-zinc-800 bg-[#111]">
        <span className="text-sm font-semibold text-zinc-500 tracking-widest uppercase">
          memex
        </span>
        <SearchBar />
      </header>

      {/* Main three-column body */}
      <div className="flex flex-1 overflow-hidden">
        <Sidebar />

        <main className="flex-1 relative overflow-hidden">
          {!state.selectedProject ? (
            <div className="flex h-full items-center justify-center text-zinc-600 text-sm">
              Select a project to explore memories
            </div>
          ) : state.memories.length === 0 && !state.loading ? (
            <div className="flex h-full items-center justify-center text-zinc-600 text-sm">
              No memories found
            </div>
          ) : (
            <GraphCanvas />
          )}

          {state.error && (
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2 bg-red-900/80 text-red-200 text-xs px-3 py-2 rounded">
              {state.error}
            </div>
          )}
        </main>

        <NodeInspector />
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Run the app and verify the full layout renders**

```bash
cd /Users/shivamvarshney/Documents/projects/memex/vision
npm run dev
```

Open `http://localhost:5173`. Expected:
- Topbar with "memex" + search box
- Left sidebar with project list (fetched from backend)
- Dark canvas center
- Selecting a project loads memories and renders the graph
- Clicking a node slides the inspector in from the right

- [ ] **Step 4: Run tests to ensure nothing broke**

```bash
npx vitest run
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/src/App.tsx vision/src/main.tsx
git commit -m "feat(vision): wire root App layout — full graph UI working"
```

---

## Task 15: `tsconfig.json` — Ensure strict TypeScript

**Files:**
- Modify: `memex/vision/tsconfig.app.json`

- [ ] **Step 1: Verify TypeScript compiles cleanly**

```bash
cd /Users/shivamvarshney/Documents/projects/memex/vision
npx tsc --noEmit
```

Expected: no errors. If there are errors, fix them before committing.

- [ ] **Step 2: Run the full build**

```bash
npm run build
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/shivamvarshney/Documents/projects/memex
git add vision/
git commit -m "feat(vision): passing TypeScript build — graph MVP complete"
```

---

## Self-Review

After writing the plan, checking it against the spec:

**Spec coverage:**
- [x] Three-column layout: topbar + sidebar + canvas + inspector — Task 14
- [x] Dark `#0d0d0d` background — Task 10 (GraphCanvas style prop) + Task 1 (index.css)
- [x] React Flow force-directed graph — Task 7 (layout.ts) + Task 10 (GraphCanvas)
- [x] Memory nodes as circles, color by type, size by importance — Tasks 4, 5, 9
- [x] Node click → inspector panel slide-in from right — Tasks 11, 14
- [x] White ring glow on selected node — Task 9 (MemoryNode boxShadow)
- [x] Unrelated nodes dim to 20% opacity — Task 10 (GraphCanvas visibleNodes)
- [x] Topbar search filters visible nodes — Tasks 13, 10
- [x] Sidebar project list — Task 12
- [x] Sidebar memory_type + topic filters — Task 12
- [x] Three edge layers (topic default on, KG + semantic lazy) — Tasks 6, 8
- [x] Edge layer toggles in sidebar — Task 12
- [x] Edge cache — Task 8 (store.tsx)
- [x] Framer Motion: inspector slide-in — Task 11
- [x] API: fetchProjects, fetchMemories, fetchSimilarMemories, fetchFacts — Task 3
- [x] Remove memex/ui — Task 2
- [x] Vite proxy to localhost:8765 — Task 1

**No gaps found.**
