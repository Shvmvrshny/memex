// All paths are relative — Vite proxies them to http://localhost:8765

export interface Memory {
  id: string
  text: string
  project: string
  topic: string
  memory_type: string
  source: string
  timestamp: string
  importance: number
  tags: string[]
  last_accessed: string
  score?: number
}

export interface Fact {
  id: string
  subject: string
  predicate: string
  object: string
  valid_from?: string
  valid_until?: string
  source?: string
  created_at: string
}

export async function fetchProjects(): Promise<string[]> {
  const res = await fetch('/trace/projects')
  if (!res.ok) throw new Error('Failed to fetch projects')
  return res.json()
}

export async function fetchMemories(
  project: string,
  memoryType?: string | null,
  topic?: string | null,
): Promise<Memory[]> {
  const params = new URLSearchParams({ project, limit: '200' })
  if (memoryType) params.set('memory_type', memoryType)
  if (topic) params.set('topic', topic)
  const res = await fetch(`/memories?${params}`)
  if (!res.ok) throw new Error('Failed to fetch memories')
  const data = await res.json()
  return data.memories ?? []
}

export async function fetchSimilarMemories(
  text: string,
  project: string,
): Promise<Memory[]> {
  const params = new URLSearchParams({ text, project, limit: '8' })
  const res = await fetch(`/memories/similar?${params}`)
  if (!res.ok) throw new Error('Failed to fetch similar memories')
  const data = await res.json()
  return data.memories ?? []
}

export async function fetchFacts(subject: string): Promise<Fact[]> {
  const params = new URLSearchParams({ subject })
  const res = await fetch(`/facts?${params}`)
  if (!res.ok) throw new Error('Failed to fetch facts')
  const data = await res.json()
  return Array.isArray(data) ? data : []
}
