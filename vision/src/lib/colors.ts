export const MEMORY_TYPE_COLORS: Record<string, string> = {
  decision:   '#6366f1',
  preference: '#8b5cf6',
  event:      '#3b82f6',
  discovery:  '#10b981',
  advice:     '#f59e0b',
  problem:    '#ef4444',
  context:    '#6b7280',
  procedure:  '#06b6d4',
  rationale:  '#ec4899',
}

export const DEFAULT_NODE_COLOR = '#6b7280'

/** Map importance [0, 1] → radius [6, 20] px */
export function importanceToRadius(importance: number): number {
  const clamped = Math.max(0, Math.min(1, importance))
  return 6 + clamped * 14
}
