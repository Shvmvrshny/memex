import { useAppStore } from './store'
import { Sidebar } from './components/Sidebar'
import { GraphCanvas } from './components/GraphCanvas'
import { NodeInspector } from './components/NodeInspector'
import { SearchBar } from './components/SearchBar'

export default function App() {
  const { state } = useAppStore()

  return (
    <div className="flex flex-col h-screen bg-[#0d0d0d] text-zinc-100 overflow-hidden">
      {/* Topbar */}
      <header className="flex-shrink-0 flex items-center justify-between px-4 py-2 border-b border-zinc-800 bg-[#111]">
        <span className="text-sm font-semibold text-zinc-500 tracking-widest uppercase">
          memex
        </span>
        <SearchBar />
      </header>

      {/* Main three-column body */}
      <div className="flex flex-1 overflow-hidden">
        <Sidebar />

        <main className="flex-1 relative overflow-hidden">
          {!state.selectedProject ? (
            <div className="flex h-full items-center justify-center text-zinc-600 text-sm">
              Select a project to explore memories
            </div>
          ) : state.loading ? (
            <div className="flex h-full items-center justify-center text-zinc-600 text-sm">
              Loading…
            </div>
          ) : state.nodes.length === 0 ? (
            <div className="flex h-full items-center justify-center text-zinc-600 text-sm">
              No memories found
            </div>
          ) : (
            <GraphCanvas key={state.selectedProject} />
          )}

          {state.error && (
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2 bg-red-900/80 text-red-200 text-xs px-3 py-2 rounded">
              {state.error}
            </div>
          )}
        </main>

        <NodeInspector />
      </div>
    </div>
  )
}
