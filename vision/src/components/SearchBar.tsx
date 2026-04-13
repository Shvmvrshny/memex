import { useAppStore } from '../store'

export function SearchBar() {
  const { state, dispatch } = useAppStore()

  return (
    <input
      type="search"
      value={state.searchQuery}
      onChange={(e) => dispatch({ type: 'SET_SEARCH', query: e.target.value })}
      placeholder="Search memories…"
      className={[
        'w-64 px-3 py-1.5 text-sm rounded-md',
        'bg-zinc-800 border border-zinc-700',
        'text-zinc-200 placeholder-zinc-500',
        'focus:outline-none focus:border-zinc-500',
        'transition-colors',
      ].join(' ')}
    />
  )
}
