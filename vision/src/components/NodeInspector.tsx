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
