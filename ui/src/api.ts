export interface Session {
  session_id: string
  project: string
  start_time: string
  tool_count: number
  skill: string
}

export interface TraceEvent {
  id: string
  session_id: string
  project: string
  turn_index: number
  tool: string
  input: string
  output: string
  reasoning: string
  duration_ms: number
  timestamp: string
  skill: string
}

const BASE = ''

export async function fetchProjects(): Promise<string[]> {
  const res = await fetch(`${BASE}/projects`)
  if (!res.ok) throw new Error('Failed to fetch projects')
  return res.json()
}

export async function fetchSessions(project: string): Promise<Session[]> {
  const res = await fetch(`${BASE}/trace/sessions?project=${encodeURIComponent(project)}`)
  if (!res.ok) throw new Error('Failed to fetch sessions')
  return res.json()
}

export async function fetchSessionEvents(sessionId: string): Promise<TraceEvent[]> {
  const res = await fetch(`${BASE}/trace/session/${encodeURIComponent(sessionId)}`)
  if (!res.ok) throw new Error('Failed to fetch session events')
  return res.json()
}
