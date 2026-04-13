import type { Edge as FlowEdge } from '@xyflow/react'
import type { Memory, Fact } from '../api'

/** Deterministic color from predicate string using a small palette */
function predicateColor(predicate: string): string {
  const palette = [
    '#6366f1', '#8b5cf6', '#3b82f6', '#10b981',
    '#f59e0b', '#ef4444', '#06b6d4', '#ec4899',
  ]
  let hash = 0
  for (let i = 0; i < predicate.length; i++) {
    hash = (hash * 31 + predicate.charCodeAt(i)) >>> 0
  }
  return palette[hash % palette.length]
}

export function buildTopicEdges(memories: Memory[]): FlowEdge[] {
  const edges: FlowEdge[] = []
  const seen = new Set<string>()

  for (let i = 0; i < memories.length; i++) {
    for (let j = i + 1; j < memories.length; j++) {
      const a = memories[i]
      const b = memories[j]
      const shareTopic = Boolean(a.topic && b.topic && a.topic === b.topic)
      const aTags = a.tags ?? []
      const bTags = b.tags ?? []
      const shareTag = aTags.length > 0 && bTags.length > 0 && aTags.some((t) => bTags.includes(t))
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
          style: { stroke: predicateColor(fact.predicate), strokeWidth: 1 },
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
    .map((m) => {
      const opacity = (m.score ?? 0.5).toFixed(2)
      return {
        id: `sim-${sourceMemory.id}-${m.id}`,
        source: sourceMemory.id,
        target: m.id,
        style: {
          stroke: `rgba(16,185,129,${opacity})`,
          strokeWidth: 1,
          strokeDasharray: '4 2',
        },
      }
    })
}
