import type { TraceEvent } from '../api'
import { EventRow } from './EventRow'
import { CheckpointBanner as _CheckpointBanner } from './CheckpointBanner'

// CheckpointBanner is available for future use when checkpoint events are rendered
void _CheckpointBanner

interface Props {
  events: TraceEvent[]
  sessionId: string
}

function buildToolSummary(events: TraceEvent[]) {
  const counts: Record<string, number> = {}
  for (const e of events) {
    counts[e.tool] = (counts[e.tool] ?? 0) + 1
  }
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .map(([tool, n]) => `${tool} ×${n}`)
    .join('  ')
}

function formatDateTime(iso: string) {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

export function TraceTimeline({ events, sessionId }: Props) {
  if (events.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-zinc-600 text-sm">
        No events in this session
      </div>
    )
  }

  const startTime = events[0]?.timestamp ?? ''
  const toolSummary = buildToolSummary(events)

  return (
    <div className="p-4 max-w-4xl">
      <div className="mb-4 pb-3 border-b border-zinc-800">
        <div className="text-xs text-zinc-500 font-mono">{sessionId}</div>
        <div className="text-sm text-zinc-300 mt-1">{formatDateTime(startTime)} · {events.length} tool calls</div>
        <div className="text-xs text-zinc-600 mt-0.5">{toolSummary}</div>
      </div>
      {events.map(event => (
        <EventRow key={event.id} event={event} />
      ))}
    </div>
  )
}
