import { useEffect, useState } from 'react'
import { fetchProjects, fetchSessions, fetchSessionEvents } from './api'
import type { Session, TraceEvent } from './api'
import { ProjectList } from './components/ProjectList'
import { SessionList } from './components/SessionList'
import { TraceTimeline } from './components/TraceTimeline'

export default function App() {
  const [projects, setProjects] = useState<string[]>([])
  const [selectedProject, setSelectedProject] = useState<string | null>(null)
  const [sessions, setSessions] = useState<Session[]>([])
  const [selectedSession, setSelectedSession] = useState<string | null>(null)
  const [events, setEvents] = useState<TraceEvent[]>([])

  useEffect(() => {
    fetchProjects().then(setProjects).catch(console.error)
  }, [])

  useEffect(() => {
    if (!selectedProject) return
    fetchSessions(selectedProject).then(setSessions).catch(console.error)
    setSelectedSession(null)
    setEvents([])
  }, [selectedProject])

  useEffect(() => {
    if (!selectedSession) return
    fetchSessionEvents(selectedSession).then(events => {
      const sorted = [...events].sort((a, b) => a.turn_index - b.turn_index)
      setEvents(sorted)
    }).catch(console.error)
  }, [selectedSession])

  return (
    <div className="flex h-screen bg-zinc-900 text-zinc-100 font-mono text-sm overflow-hidden">
      <div className="w-56 flex-shrink-0 border-r border-zinc-800 overflow-y-auto p-2">
        <div className="text-base font-semibold text-white px-2 py-2 mb-2">memex tracer</div>
        <ProjectList
          projects={projects}
          selected={selectedProject}
          onSelect={setSelectedProject}
        />
        {selectedProject && (
          <SessionList
            sessions={sessions}
            selected={selectedSession}
            onSelect={setSelectedSession}
          />
        )}
      </div>
      <div className="flex-1 overflow-y-auto">
        {!selectedSession ? (
          <div className="flex items-center justify-center h-full text-zinc-600">
            {selectedProject ? 'Select a session' : 'Select a project'}
          </div>
        ) : (
          <TraceTimeline events={events} sessionId={selectedSession} />
        )}
      </div>
    </div>
  )
}
