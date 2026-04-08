import type { Session } from '../api'

interface Props {
  sessions: Session[]
  selected: string | null
  onSelect: (sessionId: string) => void
}

function formatTime(iso: string) {
  const d = new Date(iso)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    + ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
}

export function SessionList({ sessions, selected, onSelect }: Props) {
  const sorted = [...sessions].sort(
    (a, b) => new Date(b.start_time).getTime() - new Date(a.start_time).getTime()
  )
  return (
    <div className="space-y-1">
      <div className="text-xs font-semibold text-zinc-500 uppercase tracking-wider px-2 py-1 mt-4">
        Sessions
      </div>
      {sorted.length === 0 && (
        <div className="text-xs text-zinc-600 px-2 py-1">No sessions yet</div>
      )}
      {sorted.map(s => (
        <button
          key={s.session_id}
          onClick={() => onSelect(s.session_id)}
          className={`w-full text-left px-2 py-1.5 rounded transition-colors
            ${selected === s.session_id
              ? 'bg-zinc-700 text-white'
              : 'text-zinc-400 hover:bg-zinc-800'
            }`}
        >
          <div className="text-xs font-mono">{formatTime(s.start_time)}</div>
          <div className="text-xs text-zinc-500">{s.tool_count} tool calls{s.skill ? ` · ${s.skill}` : ''}</div>
        </button>
      ))}
    </div>
  )
}
