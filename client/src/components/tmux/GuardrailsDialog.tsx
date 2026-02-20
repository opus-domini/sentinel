import { useCallback, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import type {
  GuardrailAudit,
  GuardrailAuditResponse,
  GuardrailRule,
  GuardrailRulesResponse,
} from '@/types'
import GuardrailsHelpDialog from '@/components/GuardrailsHelpDialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_GUARDRAILS_AUDIT_QUERY_KEY,
  OPS_GUARDRAILS_QUERY_KEY,
} from '@/lib/opsQueryCache'
import { cn } from '@/lib/utils'

type Tab = 'rules' | 'audit'

type GuardrailsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const KNOWN_ACTIONS = [
  'session.create',
  'session.kill',
  'window.create',
  'window.kill',
  'pane.kill',
  'pane.split',
] as const

const ACTION_LABELS: Record<string, string> = {
  'session.create': 'Create Session',
  'session.kill': 'Kill Session',
  'window.create': 'Create Window',
  'window.kill': 'Kill Window',
  'pane.kill': 'Kill Pane',
  'pane.split': 'Split Pane',
}

function actionsToPattern(actions: Array<string>): string {
  if (actions.length === 0) return ''
  if (actions.length === 1) return `^${actions[0].replace(/\./g, '\\.')}$`
  return `^(${actions.map((a) => a.replace(/\./g, '\\.')).join('|')})$`
}

function patternToActions(pattern: string): Array<string> {
  try {
    const re = new RegExp(pattern)
    return KNOWN_ACTIONS.filter((a) => re.test(a))
  } catch {
    return []
  }
}

type RuleDraft = {
  name: string
  actions: Array<string>
  mode: GuardrailRule['mode']
  severity: GuardrailRule['severity']
  message: string
  enabled: boolean
  priority: number
}

const defaultNewRule: RuleDraft = {
  name: '',
  actions: [],
  mode: 'warn',
  severity: 'warn',
  message: '',
  enabled: true,
  priority: 100,
}

const labelClass =
  'text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground'

const modeOptions = [
  { value: 'warn', label: 'Warn' },
  { value: 'confirm', label: 'Confirm' },
  { value: 'block', label: 'Block' },
] as const

const modeDescription: Record<string, string> = {
  warn: 'Log the match and allow execution to proceed',
  confirm: 'Require explicit confirmation before execution',
  block: 'Deny execution entirely',
}

function formatAuditTime(raw: string): string {
  if (!raw) return '-'
  const d = new Date(raw)
  if (isNaN(d.getTime())) return raw
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function decisionBadgeClass(mode: string): string {
  switch (mode) {
    case 'block':
      return 'border-destructive/45 bg-destructive/10 text-destructive-foreground'
    case 'confirm':
      return 'border-amber-500/45 bg-amber-500/10 text-amber-600 dark:text-amber-400'
    case 'warn':
      return 'border-amber-500/45 bg-amber-500/10 text-amber-600 dark:text-amber-400'
    default:
      return 'border-ok/45 bg-ok/10 text-ok-foreground'
  }
}

function severityBadgeClass(severity: string): string {
  switch (severity) {
    case 'error':
      return 'border-destructive/45 bg-destructive/10 text-destructive-foreground'
    case 'warn':
      return 'border-amber-500/45 bg-amber-500/10 text-amber-600 dark:text-amber-400'
    default:
      return 'border-border-subtle bg-surface-overlay text-muted-foreground'
  }
}

type RuleFormProps = {
  title: string
  draft: RuleDraft
  onDraftChange: Dispatch<SetStateAction<RuleDraft>>
  advanced: boolean
  onAdvancedChange: Dispatch<SetStateAction<boolean>>
  onSubmit: () => void
  onCancel: () => void
  submitLabel: string
  submittingLabel: string
  submitting: boolean
}

function RuleForm({
  title,
  draft,
  onDraftChange,
  advanced,
  onAdvancedChange,
  onSubmit,
  onCancel,
  submitLabel,
  submittingLabel,
  submitting,
}: RuleFormProps) {
  return (
    <div className="rounded border border-primary/30 bg-surface-elevated p-3">
      <h4 className="mb-3 text-[12px] font-semibold">{title}</h4>
      <div className="grid gap-3">
        <div>
          <label className={labelClass}>
            Actions <span className="text-destructive-foreground">*</span>
          </label>
          <div className="mt-1 flex flex-wrap gap-1">
            {KNOWN_ACTIONS.map((action) => (
              <button
                key={action}
                type="button"
                onClick={() =>
                  onDraftChange((prev) => ({
                    ...prev,
                    actions: prev.actions.includes(action)
                      ? prev.actions.filter((a) => a !== action)
                      : [...prev.actions, action],
                  }))
                }
                className={cn(
                  'rounded-md border px-2 py-1 text-[11px] font-medium transition-colors',
                  draft.actions.includes(action)
                    ? 'border-primary/40 bg-primary/15 text-primary-text'
                    : 'border-border-subtle bg-surface-overlay text-muted-foreground hover:text-foreground',
                )}
              >
                {ACTION_LABELS[action]}
              </button>
            ))}
          </div>
          <p className="mt-1 text-[10px] text-muted-foreground">
            Sentinel UI operations this rule will intercept
          </p>
        </div>

        <div>
          <label className={labelClass}>Mode</label>
          <div className="mt-1 flex gap-1 rounded-md border border-border-subtle bg-secondary p-1">
            {modeOptions.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() =>
                  onDraftChange((prev) => ({ ...prev, mode: opt.value }))
                }
                className={cn(
                  'flex-1 rounded-md px-2 py-1.5 text-[11px] font-medium transition-colors',
                  draft.mode === opt.value
                    ? 'bg-primary/15 text-primary-text'
                    : 'text-muted-foreground hover:bg-surface-overlay hover:text-foreground',
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <p className="mt-1 text-[10px] text-muted-foreground">
            {modeDescription[draft.mode]}
          </p>
        </div>

        <div>
          <label className={labelClass}>Message</label>
          <Input
            type="text"
            placeholder="Dangerous operation detected"
            value={draft.message}
            onChange={(e) =>
              onDraftChange((prev) => ({ ...prev, message: e.target.value }))
            }
            className="mt-0.5 h-8 bg-surface-overlay text-[12px]"
          />
          <p className="mt-1 text-[10px] text-muted-foreground">
            Shown to the user when the rule triggers
          </p>
        </div>

        <div>
          <label className={labelClass}>Name</label>
          <Input
            type="text"
            placeholder="Block destructive operations"
            value={draft.name}
            onChange={(e) =>
              onDraftChange((prev) => ({ ...prev, name: e.target.value }))
            }
            className="mt-0.5 h-8 bg-surface-overlay text-[12px]"
          />
          <p className="mt-1 text-[10px] text-muted-foreground">
            Optional friendly label
          </p>
        </div>

        <div>
          <button
            type="button"
            onClick={() => onAdvancedChange((prev) => !prev)}
            className="flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground"
          >
            <ChevronRight
              className={cn(
                'h-3 w-3 transition-transform',
                advanced && 'rotate-90',
              )}
            />
            Advanced options
          </button>
          {advanced && (
            <div className="mt-2 grid gap-3 sm:grid-cols-2">
              <div>
                <label className={labelClass}>Severity</label>
                <Select
                  value={draft.severity}
                  onValueChange={(v) =>
                    onDraftChange((prev) => ({
                      ...prev,
                      severity: v as GuardrailRule['severity'],
                    }))
                  }
                >
                  <SelectTrigger className="mt-0.5 h-8 bg-surface-overlay text-[12px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="info">info</SelectItem>
                    <SelectItem value="warn">warn</SelectItem>
                    <SelectItem value="error">error</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className={labelClass}>Priority</label>
                <Input
                  type="number"
                  value={draft.priority}
                  onChange={(e) =>
                    onDraftChange((prev) => ({
                      ...prev,
                      priority: parseInt(e.target.value, 10) || 0,
                    }))
                  }
                  className="mt-0.5 h-8 bg-surface-overlay text-[12px]"
                />
                <p className="mt-1 text-[10px] text-muted-foreground">
                  Lower value = higher priority. Default: 100
                </p>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="mt-3 flex items-center gap-2">
        <Button
          size="sm"
          className="h-7 text-[11px]"
          onClick={onSubmit}
          disabled={submitting}
        >
          {submitting ? submittingLabel : submitLabel}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-7 text-[11px]"
          onClick={onCancel}
        >
          Cancel
        </Button>
      </div>
    </div>
  )
}

export default function GuardrailsDialog({
  open,
  onOpenChange,
}: GuardrailsDialogProps) {
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<Tab>('rules')
  const [savingID, setSavingID] = useState('')
  const [deletingID, setDeletingID] = useState('')
  const [showAddForm, setShowAddForm] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [newRule, setNewRule] = useState(defaultNewRule)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<RuleDraft>(defaultNewRule)
  const [editAdvanced, setEditAdvanced] = useState(false)

  const rulesQuery = useQuery({
    queryKey: OPS_GUARDRAILS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<GuardrailRulesResponse>(
        '/api/ops/guardrails/rules',
      )
      return data.rules
    },
    enabled: open,
  })

  const auditQuery = useQuery({
    queryKey: OPS_GUARDRAILS_AUDIT_QUERY_KEY,
    queryFn: async () => {
      const data = await api<GuardrailAuditResponse>(
        '/api/ops/guardrails/audit?limit=100',
      )
      return data.audit
    },
    enabled: open && activeTab === 'audit',
  })

  const rules = rulesQuery.data ?? []
  const audit = auditQuery.data ?? []
  const rulesLoading = rulesQuery.isLoading
  const auditLoading = auditQuery.isLoading

  const refreshPage = useCallback(() => {
    if (activeTab === 'rules') {
      void queryClient.refetchQueries({
        queryKey: OPS_GUARDRAILS_QUERY_KEY,
        exact: true,
      })
    } else {
      void queryClient.refetchQueries({
        queryKey: OPS_GUARDRAILS_AUDIT_QUERY_KEY,
        exact: true,
      })
    }
  }, [activeTab, queryClient])

  const saveRule = useCallback(
    async (
      rule: GuardrailRule,
      patch: Partial<GuardrailRule>,
    ): Promise<boolean> => {
      setSavingID(rule.id)
      setError('')
      try {
        await api<GuardrailRulesResponse>(
          `/api/ops/guardrails/rules/${encodeURIComponent(rule.id)}`,
          {
            method: 'PATCH',
            body: JSON.stringify({
              name: patch.name ?? rule.name,
              scope: 'action',
              pattern: patch.pattern ?? rule.pattern,
              mode: patch.mode ?? rule.mode,
              severity: patch.severity ?? rule.severity,
              message: patch.message ?? rule.message,
              enabled: patch.enabled ?? rule.enabled,
              priority: patch.priority ?? rule.priority,
            }),
          },
        )
        await queryClient.invalidateQueries({
          queryKey: OPS_GUARDRAILS_QUERY_KEY,
          exact: true,
        })
        return true
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'failed to update guardrail',
        )
        return false
      } finally {
        setSavingID('')
      }
    },
    [api, queryClient],
  )

  const createRule = useCallback(async () => {
    if (newRule.actions.length === 0) {
      setError('At least one action is required')
      return
    }
    setCreating(true)
    setError('')
    try {
      await api<GuardrailRulesResponse>('/api/ops/guardrails/rules', {
        method: 'POST',
        body: JSON.stringify({
          name: newRule.name,
          scope: 'action',
          pattern: actionsToPattern(newRule.actions),
          mode: newRule.mode,
          severity: newRule.severity,
          message: newRule.message,
          enabled: newRule.enabled,
          priority: newRule.priority,
        }),
      })
      await queryClient.invalidateQueries({
        queryKey: OPS_GUARDRAILS_QUERY_KEY,
        exact: true,
      })
      setNewRule(defaultNewRule)
      setShowAddForm(false)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'failed to create guardrail rule',
      )
    } finally {
      setCreating(false)
    }
  }, [api, newRule, queryClient])

  const deleteRule = useCallback(
    async (ruleId: string) => {
      setDeletingID(ruleId)
      setError('')
      try {
        await api<{ removed: string }>(
          `/api/ops/guardrails/rules/${encodeURIComponent(ruleId)}`,
          { method: 'DELETE' },
        )
        await queryClient.invalidateQueries({
          queryKey: OPS_GUARDRAILS_QUERY_KEY,
          exact: true,
        })
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'failed to delete guardrail',
        )
      } finally {
        setDeletingID('')
      }
    },
    [api, queryClient],
  )

  const startEdit = (rule: GuardrailRule) => {
    setEditingId(rule.id)
    setEditDraft({
      name: rule.name,
      actions: patternToActions(rule.pattern),
      mode: rule.mode,
      severity: rule.severity,
      message: rule.message,
      enabled: rule.enabled,
      priority: rule.priority,
    })
    setEditAdvanced(false)
    setShowAddForm(false)
    setShowAdvanced(false)
  }

  const cancelEdit = () => setEditingId(null)

  const saveEdit = useCallback(async () => {
    if (editDraft.actions.length === 0) {
      setError('At least one action is required')
      return
    }
    const rule = rules.find((r) => r.id === editingId)
    if (!rule) return
    const ok = await saveRule(rule, {
      ...editDraft,
      pattern: actionsToPattern(editDraft.actions),
    })
    if (ok) setEditingId(null)
  }, [editDraft, editingId, rules, saveRule])

  const tabClass = (tab: Tab) =>
    cn(
      'rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors',
      activeTab === tab
        ? 'bg-primary/15 text-primary-text'
        : 'text-muted-foreground hover:bg-surface-overlay hover:text-foreground',
    )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-[5%] flex min-h-[24rem] max-h-[88vh] -translate-y-0 flex-col overflow-hidden sm:min-h-[38rem] sm:max-w-6xl">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <DialogTitle>Guardrails</DialogTitle>
            <GuardrailsHelpDialog />
          </div>
          <DialogDescription>
            Safety rules that evaluate actions before execution.
          </DialogDescription>
        </DialogHeader>

        <section className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <nav className="flex gap-1 rounded-md border border-border-subtle bg-secondary p-1">
              <button
                type="button"
                className={tabClass('rules')}
                onClick={() => setActiveTab('rules')}
              >
                Rules
              </button>
              <button
                type="button"
                className={tabClass('audit')}
                onClick={() => setActiveTab('audit')}
              >
                Audit Log
              </button>
            </nav>
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </Button>
          </div>
          {activeTab === 'rules' && (
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 text-[11px]"
              onClick={() => {
                setShowAddForm((prev) => !prev)
                setEditingId(null)
              }}
            >
              <Plus className="h-3.5 w-3.5" />
              Add Rule
            </Button>
          )}
        </section>

        {error.trim() !== '' && (
          <div className="rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
            {error}
          </div>
        )}

        <section className="min-h-0 flex-1 overflow-y-auto rounded-lg border border-border-subtle bg-secondary">
          <div className="grid gap-1.5 p-2">
            {activeTab === 'rules' && (
              <>
                {showAddForm && (
                  <RuleForm
                    title="New Rule"
                    draft={newRule}
                    onDraftChange={setNewRule}
                    advanced={showAdvanced}
                    onAdvancedChange={setShowAdvanced}
                    onSubmit={() => void createRule()}
                    onCancel={() => {
                      setShowAddForm(false)
                      setShowAdvanced(false)
                      setNewRule(defaultNewRule)
                    }}
                    submitLabel="Create"
                    submittingLabel="Creating..."
                    submitting={creating}
                  />
                )}

                {rulesLoading &&
                  Array.from({ length: 3 }).map((_, idx) => (
                    <div
                      key={`rule-skeleton-${idx}`}
                      className="h-20 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                    />
                  ))}

                {rules.map((rule) =>
                  editingId === rule.id ? (
                    <RuleForm
                      key={rule.id}
                      title="Edit Rule"
                      draft={editDraft}
                      onDraftChange={setEditDraft}
                      advanced={editAdvanced}
                      onAdvancedChange={setEditAdvanced}
                      onSubmit={() => void saveEdit()}
                      onCancel={cancelEdit}
                      submitLabel="Save"
                      submittingLabel="Saving..."
                      submitting={savingID === rule.id}
                    />
                  ) : (
                    <div
                      key={rule.id}
                      className="min-w-0 overflow-x-hidden rounded-md border border-border-subtle bg-surface-overlay p-2"
                    >
                      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center">
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-[12px] font-medium">
                            {rule.name || rule.id}
                          </p>
                          <p className="truncate text-[11px] text-muted-foreground">
                            {rule.message || rule.id}
                          </p>
                        </div>
                        <div className="ml-auto flex w-full max-w-full min-w-0 flex-wrap items-center justify-end gap-2 sm:w-auto md:flex-nowrap">
                          <Badge
                            variant="outline"
                            className={cn(
                              'max-w-full justify-center truncate sm:w-[4rem]',
                              severityBadgeClass(rule.severity),
                            )}
                          >
                            {rule.severity}
                          </Badge>
                          <Select
                            value={rule.mode}
                            onValueChange={(value: string) => {
                              void saveRule(rule, {
                                mode: value as GuardrailRule['mode'],
                              })
                            }}
                            disabled={savingID === rule.id}
                          >
                            <SelectTrigger className="w-[min(7.25rem,42vw)] sm:w-[7.5rem]">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="warn">warn</SelectItem>
                              <SelectItem value="confirm">confirm</SelectItem>
                              <SelectItem value="block">block</SelectItem>
                            </SelectContent>
                          </Select>
                          <button
                            type="button"
                            role="switch"
                            aria-checked={rule.enabled}
                            aria-label={`${rule.name || rule.id} toggle`}
                            onClick={() => {
                              void saveRule(rule, {
                                enabled: !rule.enabled,
                              })
                            }}
                            disabled={savingID === rule.id}
                            className={cn(
                              'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors',
                              'focus-visible:ring-ring/40 focus-visible:outline-none focus-visible:ring-2',
                              'disabled:cursor-not-allowed disabled:opacity-60',
                              rule.enabled
                                ? 'border-ok/50 bg-ok/40'
                                : 'border-border-subtle bg-surface-overlay',
                            )}
                          >
                            <span
                              className={cn(
                                'inline-block h-4 w-4 rounded-full bg-white shadow transition-transform',
                                rule.enabled
                                  ? 'translate-x-5'
                                  : 'translate-x-1',
                              )}
                            />
                          </button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-foreground"
                            onClick={() => startEdit(rule)}
                            aria-label={`Edit ${rule.name || rule.id}`}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-destructive-foreground"
                            onClick={() => void deleteRule(rule.id)}
                            disabled={deletingID === rule.id}
                            aria-label={`Delete ${rule.name || rule.id}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-1">
                        {patternToActions(rule.pattern).map((action) => (
                          <span
                            key={action}
                            className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary-text"
                          >
                            {ACTION_LABELS[action] ?? action}
                          </span>
                        ))}
                        <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">
                          p:{rule.priority}
                        </span>
                      </div>
                    </div>
                  ),
                )}

                {!rulesLoading && rules.length === 0 && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>No guardrail rules configured.</p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-fit text-[11px]"
                      onClick={() => setShowAddForm(true)}
                    >
                      Add your first rule
                    </Button>
                  </div>
                )}
              </>
            )}

            {activeTab === 'audit' && (
              <>
                {auditLoading &&
                  Array.from({ length: 5 }).map((_, idx) => (
                    <div
                      key={`audit-skeleton-${idx}`}
                      className="h-16 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                    />
                  ))}

                {audit.map((entry: GuardrailAudit) => (
                  <div
                    key={entry.id}
                    className="rounded border border-border-subtle bg-surface-overlay p-2"
                  >
                    <div className="flex min-w-0 items-center justify-between gap-2">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <Badge
                            variant="outline"
                            className={cn(
                              'shrink-0',
                              decisionBadgeClass(entry.decision),
                            )}
                          >
                            {entry.decision}
                          </Badge>
                          <span className="truncate text-[12px] font-medium">
                            {entry.action || '-'}
                          </span>
                          {entry.override && (
                            <Badge
                              variant="outline"
                              className="shrink-0 border-amber-500/45 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                            >
                              override
                            </Badge>
                          )}
                        </div>
                        <div className="mt-0.5 flex flex-wrap gap-x-3 text-[10px] text-muted-foreground">
                          {entry.sessionName && (
                            <span>session: {entry.sessionName}</span>
                          )}
                          {entry.paneId && <span>pane: {entry.paneId}</span>}
                          {entry.ruleId && <span>rule: {entry.ruleId}</span>}
                          {entry.reason && <span>reason: {entry.reason}</span>}
                        </div>
                      </div>
                      <span className="shrink-0 text-[10px] text-muted-foreground">
                        {formatAuditTime(entry.createdAt)}
                      </span>
                    </div>
                  </div>
                ))}

                {!auditLoading && audit.length === 0 && (
                  <div className="grid gap-2 rounded border border-dashed border-border-subtle p-3 text-[12px] text-muted-foreground">
                    <p>No audit entries yet.</p>
                    <p>Guardrail evaluations will appear here as they occur.</p>
                  </div>
                )}
              </>
            )}
          </div>
        </section>

        <div className="flex items-center justify-between text-[11px] text-muted-foreground">
          <span>
            {activeTab === 'rules'
              ? `${rules.length} rule(s)`
              : `${audit.length} audit entries`}
          </span>
        </div>
      </DialogContent>
    </Dialog>
  )
}
