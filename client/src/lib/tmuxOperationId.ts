let tmuxOperationSequence = 0

export function createTmuxOperationId(prefix: string): string {
  const normalizedPrefix = prefix.trim() || 'tmux-op'
  tmuxOperationSequence += 1
  return `${normalizedPrefix}-${Date.now()}-${tmuxOperationSequence}`
}
