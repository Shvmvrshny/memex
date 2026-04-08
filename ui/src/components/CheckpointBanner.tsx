interface Props {
  summary: string
}

export function CheckpointBanner({ summary }: Props) {
  return (
    <div className="my-3 border border-zinc-700 rounded bg-zinc-800/50 px-3 py-2">
      <div className="text-xs text-zinc-500 mb-1">── checkpoint ──</div>
      <pre className="text-xs text-zinc-300 whitespace-pre-wrap">{summary}</pre>
    </div>
  )
}
