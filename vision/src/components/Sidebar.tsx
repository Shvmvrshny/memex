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
