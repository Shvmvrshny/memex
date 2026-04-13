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
