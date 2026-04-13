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
      const edges = deriveEdges(action.memories, state.activeEdgeLayers, state.edgeCache)
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
    )
      return

    const uniqueTopics = [
      ...new Set(state.memories.map((m) => m.topic).filter(Boolean)),
    ]
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
    )
      return

    Promise.all(
      state.memories.map((m) =>
        fetchSimilarMemories(m.text, state.selectedProject!).then((similar) =>
          buildSemanticEdges(m, similar),
        ),
      ),
    )
      .then((edgeArrays) => {
        const allEdges = edgeArrays.flat()
        // Deduplicate: keep only one edge per pair
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
