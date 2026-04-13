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
      const matchesSearch =
        !q ||
        mem.text.toLowerCase().includes(q) ||
        mem.topic.toLowerCase().includes(q)
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
    const visibleIds = new Set(
      visibleNodes.filter((n) => !n.hidden).map((n) => n.id),
    )
    return edges.filter(
      (e) =>
        visibleIds.has(e.source as string) && visibleIds.has(e.target as string),
    )
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
