import { useCallback, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Plus, RefreshCw, Shield, Trash2 } from 'lucide-react'
import type {
  GuardrailAudit,
  GuardrailAuditResponse,
  GuardrailRule,
  GuardrailRulesResponse,
} from '@/types'
import GuardrailsHelpDialog from '@/components/GuardrailsHelpDialog'
import AppShell from '@/components/layout/AppShell'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import {
  OPS_GUARDRAILS_AUDIT_QUERY_KEY,
  OPS_GUARDRAILS_QUERY_KEY,
} from '@/lib/opsQueryCache'
import { cn } from '@/lib/utils'

type Tab = 'rules' | 'audit'

const defaultNewRule = {
  name: '',
  scope: 'command' as const,
  pattern: '',
  mode: 'warn' as const,
  severity: 'warn' as const,
  message: '',
  enabled: true,
  priority: 100,
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

function GuardrailsPage() {
  const layout = useLayoutContext()
  const api = useTmuxApi()
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<Tab>('rules')
  const [savingID, setSavingID] = useState('')
  const [deletingID, setDeletingID] = useState('')
  const [showAddForm, setShowAddForm] = useState(false)
  const [newRule, setNewRule] = useState(defaultNewRule)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  const rulesQuery = useQuery({
    queryKey: OPS_GUARDRAILS_QUERY_KEY,
    queryFn: async () => {
      const data = await api<GuardrailRulesResponse>(
        '/api/ops/guardrails/rules',
      )
      return data.rules
    },
  })

  const auditQuery = useQuery({
    queryKey: OPS_GUARDRAILS_AUDIT_QUERY_KEY,
    queryFn: async () => {
      const data = await api<GuardrailAuditResponse>(
        '/api/ops/guardrails/audit?limit=100',
      )
      return data.audit
    },
    enabled: activeTab === 'audit',
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
    async (rule: GuardrailRule, patch: Partial<GuardrailRule>) => {
      setSavingID(rule.id)
      setError('')
      try {
        await api<GuardrailRulesResponse>(
          `/api/ops/guardrails/rules/${encodeURIComponent(rule.id)}`,
          {
            method: 'PATCH',
            body: JSON.stringify({
              name: patch.name ?? rule.name,
              scope: patch.scope ?? rule.scope,
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
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'failed to update guardrail',
        )
      } finally {
        setSavingID('')
      }
    },
    [api, queryClient],
  )

  const createRule = useCallback(async () => {
    if (!newRule.pattern.trim()) {
      setError('Pattern is required')
      return
    }
    setCreating(true)
    setError('')
    try {
      await api<GuardrailRulesResponse>('/api/ops/guardrails/rules', {
        method: 'POST',
        body: JSON.stringify({
          name: newRule.name,
          scope: newRule.scope,
          pattern: newRule.pattern,
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

  const tabClass = (tab: Tab) =>
    cn(
      'rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors',
      activeTab === tab
        ? 'bg-primary/15 text-primary-text'
        : 'text-muted-foreground hover:bg-surface-overlay hover:text-foreground',
    )

  return (
    <AppShell>
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-[radial-gradient(circle_at_20%_-10%,rgba(99,102,241,.16),transparent_34%),var(--background)]">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Shield className="h-5 w-5" />
            </Button>
            <span className="truncate">Sentinel</span>
            <span className="text-muted-foreground">/</span>
            <span className="truncate text-muted-foreground">guardrails</span>
          </div>
          <div className="flex items-center gap-1.5">
            <GuardrailsHelpDialog />
            <Button
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
              onClick={refreshPage}
              aria-label="Refresh guardrails"
            >
              <RefreshCw className="h-4 w-4" />
              Refresh
            </Button>
          </div>
        </header>

        <section className="grid min-h-0 grid-rows-[auto_1fr] gap-2 overflow-hidden p-3">
          <div className="flex items-center justify-between gap-2">
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
            {activeTab === 'rules' && (
              <Button
                variant="outline"
                size="sm"
                className="h-7 gap-1 text-[11px]"
                onClick={() => setShowAddForm((prev) => !prev)}
              >
                <Plus className="h-3.5 w-3.5" />
                Add Rule
              </Button>
            )}
          </div>

          {error.trim() !== '' && (
            <div className="rounded border border-destructive/45 bg-destructive/10 px-2 py-1 text-[11px] text-destructive-foreground">
              {error}
            </div>
          )}

          <div className="grid min-h-0 grid-rows-[1fr] overflow-hidden rounded-lg border border-border-subtle bg-secondary">
            <ScrollArea className="h-full min-h-0">
              <div className="grid gap-1.5 p-2">
                {activeTab === 'rules' && (
                  <>
                    {showAddForm && (
                      <div className="rounded border border-primary/30 bg-surface-elevated p-3">
                        <h4 className="mb-2 text-[12px] font-semibold">
                          New Rule
                        </h4>
                        <div className="grid gap-2 sm:grid-cols-2">
                          <input
                            type="text"
                            placeholder="Name"
                            value={newRule.name}
                            onChange={(e) =>
                              setNewRule((prev) => ({
                                ...prev,
                                name: e.target.value,
                              }))
                            }
                            className="rounded border border-border-subtle bg-background px-2 py-1 text-[12px] outline-none focus:border-primary"
                          />
                          <input
                            type="text"
                            placeholder="Pattern (regex) *"
                            value={newRule.pattern}
                            onChange={(e) =>
                              setNewRule((prev) => ({
                                ...prev,
                                pattern: e.target.value,
                              }))
                            }
                            className="rounded border border-border-subtle bg-background px-2 py-1 font-mono text-[12px] outline-none focus:border-primary"
                          />
                          <Select
                            value={newRule.scope}
                            onValueChange={(v) =>
                              setNewRule((prev) => ({
                                ...prev,
                                scope: v as 'action' | 'command',
                              }))
                            }
                          >
                            <SelectTrigger className="text-[12px]">
                              <SelectValue placeholder="Scope" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="command">command</SelectItem>
                              <SelectItem value="action">action</SelectItem>
                            </SelectContent>
                          </Select>
                          <Select
                            value={newRule.mode}
                            onValueChange={(v) =>
                              setNewRule((prev) => ({
                                ...prev,
                                mode: v as GuardrailRule['mode'],
                              }))
                            }
                          >
                            <SelectTrigger className="text-[12px]">
                              <SelectValue placeholder="Mode" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="warn">warn</SelectItem>
                              <SelectItem value="confirm">confirm</SelectItem>
                              <SelectItem value="block">block</SelectItem>
                            </SelectContent>
                          </Select>
                          <Select
                            value={newRule.severity}
                            onValueChange={(v) =>
                              setNewRule((prev) => ({
                                ...prev,
                                severity: v as GuardrailRule['severity'],
                              }))
                            }
                          >
                            <SelectTrigger className="text-[12px]">
                              <SelectValue placeholder="Severity" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="info">info</SelectItem>
                              <SelectItem value="warn">warn</SelectItem>
                              <SelectItem value="error">error</SelectItem>
                            </SelectContent>
                          </Select>
                          <input
                            type="number"
                            placeholder="Priority"
                            value={newRule.priority}
                            onChange={(e) =>
                              setNewRule((prev) => ({
                                ...prev,
                                priority: parseInt(e.target.value, 10) || 0,
                              }))
                            }
                            className="rounded border border-border-subtle bg-background px-2 py-1 text-[12px] outline-none focus:border-primary"
                          />
                        </div>
                        <input
                          type="text"
                          placeholder="Message"
                          value={newRule.message}
                          onChange={(e) =>
                            setNewRule((prev) => ({
                              ...prev,
                              message: e.target.value,
                            }))
                          }
                          className="mt-2 w-full rounded border border-border-subtle bg-background px-2 py-1 text-[12px] outline-none focus:border-primary"
                        />
                        <div className="mt-2 flex items-center gap-2">
                          <Button
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => void createRule()}
                            disabled={creating}
                          >
                            {creating ? 'Creating...' : 'Create'}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-[11px]"
                            onClick={() => {
                              setShowAddForm(false)
                              setNewRule(defaultNewRule)
                            }}
                          >
                            Cancel
                          </Button>
                        </div>
                      </div>
                    )}

                    {rulesLoading &&
                      Array.from({ length: 3 }).map((_, idx) => (
                        <div
                          key={`rule-skeleton-${idx}`}
                          className="h-20 animate-pulse rounded border border-border-subtle bg-surface-elevated"
                        />
                      ))}

                    {rules.map((rule) => (
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
                              className="max-w-full justify-center truncate sm:w-[5.5rem]"
                            >
                              {rule.scope}
                            </Badge>
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
                              className="h-7 w-7 text-muted-foreground hover:text-destructive-foreground"
                              onClick={() => void deleteRule(rule.id)}
                              disabled={deletingID === rule.id}
                              aria-label={`Delete ${rule.name || rule.id}`}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </div>
                        <div className="mt-1 flex items-center gap-2">
                          <code className="truncate text-[11px] text-muted-foreground">
                            {rule.pattern}
                          </code>
                          <span className="shrink-0 text-[10px] text-muted-foreground">
                            p:{rule.priority}
                          </span>
                        </div>
                      </div>
                    ))}

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
                                {entry.action || entry.command || '-'}
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
                              {entry.paneId && (
                                <span>pane: {entry.paneId}</span>
                              )}
                              {entry.ruleId && (
                                <span>rule: {entry.ruleId}</span>
                              )}
                              {entry.reason && (
                                <span>reason: {entry.reason}</span>
                              )}
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
                        <p>
                          Guardrail evaluations will appear here as they occur.
                        </p>
                      </div>
                    )}
                  </>
                )}
              </div>
            </ScrollArea>
          </div>
        </section>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">
            {activeTab === 'rules'
              ? `${rules.length} rule(s)`
              : `${audit.length} audit entries`}
          </span>
          <span className="shrink-0 whitespace-nowrap">
            {activeTab === 'rules' ? 'Rules' : 'Audit'}
          </span>
        </footer>
      </main>
    </AppShell>
  )
}

export const Route = createFileRoute('/guardrails')({
  component: GuardrailsPage,
})
