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
 * Call this after fetching memories; React Flow renders from those positions.
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
    .filter(
      (e) =>
        nodeIds.has(e.source as string) && nodeIds.has(e.target as string),
    )
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
