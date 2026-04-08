import { useState } from 'react'
import type { TraceEvent } from '../api'

const TOOL_COLORS: Record<string, string> = {
  Read:  'text-blue-400',
  Edit:  'text-yellow-400',
  Bash:  'text-green-400',
  Grep:  'text-purple-400',
  Glob:  'text-cyan-400',
  Write: 'text-orange-400',
}

function toolColor(tool: string) {
  return TOOL_COLORS[tool] ?? 'text-zinc-300'
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function formatTime(iso: string) {
  const d = new Date(iso)
  return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

interface Props {
  event: TraceEvent
}

export function EventRow({ event }: Props) {
  const [outputOpen, setOutputOpen] = useState(false)

  return (
    <div className="border-b border-zinc-800 py-2">
      {event.reasoning && (
        <div className="text-xs text-zinc-500 italic mb-1 pl-1">
          "{event.reasoning}"
        </div>
      )}
      <div className="flex items-start gap-3">
        <span className="text-xs text-zinc-600 w-20 flex-shrink-0 pt-0.5">
          {formatTime(event.timestamp)}
        </span>
        <span className={`text-xs font-semibold w-14 flex-shrink-0 ${toolColor(event.tool)}`}>
          {event.tool}
        </span>
        <span className="text-xs text-zinc-300 flex-1 truncate">{event.input}</span>
        <span className="text-xs text-zinc-600 flex-shrink-0">{formatDuration(event.duration_ms)}</span>
        {event.output && (
          <button
            onClick={() => setOutputOpen(o => !o)}
            className="text-xs text-zinc-500 hover:text-zinc-300 flex-shrink-0 transition-colors"
          >
            {outputOpen ? '▼ hide' : '▶ output'}
          </button>
        )}
      </div>
      {outputOpen && event.output && (
        <pre className="mt-1 ml-20 text-xs text-zinc-400 bg-zinc-800 rounded p-2 overflow-x-auto max-h-48 whitespace-pre-wrap">
          {event.output}
        </pre>
      )}
    </div>
  )
}
