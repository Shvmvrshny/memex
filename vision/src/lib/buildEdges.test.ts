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
