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
