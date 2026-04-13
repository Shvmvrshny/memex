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
