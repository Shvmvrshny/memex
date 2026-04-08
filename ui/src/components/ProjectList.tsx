interface Props {
  projects: string[]
  selected: string | null
  onSelect: (project: string) => void
}

export function ProjectList({ projects, selected, onSelect }: Props) {
  return (
    <div className="space-y-1">
      <div className="text-xs font-semibold text-zinc-500 uppercase tracking-wider px-2 py-1">
        Projects
      </div>
      {projects.length === 0 && (
        <div className="text-xs text-zinc-600 px-2 py-1">No projects yet</div>
      )}
      {projects.map(p => (
        <button
          key={p}
          onClick={() => onSelect(p)}
          className={`w-full text-left px-2 py-1 rounded text-sm font-mono truncate transition-colors
            ${selected === p
              ? 'bg-zinc-700 text-white'
              : 'text-zinc-300 hover:bg-zinc-800'
            }`}
        >
          {p}
        </button>
      ))}
    </div>
  )
}
