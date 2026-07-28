import { useCallback, useEffect, useRef } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import type { OpsRunbook, OpsRunbookRun } from '@/types'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { RunbookDeleteDialog } from '@/components/RunbookDeleteDialog'
import { RunbookEditor } from '@/components/RunbookEditor'
import { RunbookRunDialog } from '@/components/RunbookRunDialog'
import { RunbookDetailPanel } from '@/components/runbooks/RunbookDetailPanel'
import { RunbookExecutionReceipt } from '@/components/runbooks/RunbookExecutionReceipt'
import { RunbookJobHistory } from '@/components/runbooks/RunbookJobHistory'
import { RunbookOperationsSummary } from '@/components/runbooks/RunbookOperationsSummary'
import RunbooksSidebar from '@/components/RunbooksSidebar'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useRunbooksPage } from '@/hooks/useRunbooksPage'
import {
  parseRunbooksSearch,
  runbookDefinitionSearch,
  runbookExecutionSearch,
} from '@/lib/deepLinks'
import type { RunbooksSearch } from '@/lib/deepLinks'

type RunbooksDeepLinkResolution =
  | { kind: 'execution'; key: string; jobId: string; selectedRunbookId: string | null }
  | { kind: 'definition'; key: string; selectedRunbookId: string }
  | { kind: 'invalid' }
  | { kind: 'none' }

export function resolveRunbooksDeepLink(
  search: RunbooksSearch,
  runbooks: Array<OpsRunbook>,
  focusedJob: OpsRunbookRun | null,
): RunbooksDeepLinkResolution {
  if (search.job != null) {
    if (focusedJob == null) return { kind: 'invalid' }
    const runbook = runbooks.find((item) => item.id === focusedJob.runbookId)
    return {
      kind: 'execution',
      key: `job:${focusedJob.id}:${runbook?.id ?? 'standalone'}`,
      jobId: focusedJob.id,
      selectedRunbookId: runbook?.id ?? null,
    }
  }
  if (search.runbook != null) {
    const runbook = runbooks.find((item) => item.id === search.runbook)
    if (runbook == null) return { kind: 'invalid' }
    return {
      kind: 'definition',
      key: `runbook:${runbook.id}`,
      selectedRunbookId: runbook.id,
    }
  }
  return { kind: 'none' }
}

export function RunbooksPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const { hostname } = useMetaContext()
  const layout = useLayoutContext()

  const {
    runbooks,
    jobs,
    schedules,
    runbooksLoading,
    focusedJobLoading,
    focusedJobError,
    focusedJob,
    targetServices,
    targetServicesLoading,
    connectionState,
    selectedRunbookId,
    selectedRunbook,
    selectedJobs,
    selectedSchedule,
    editingDraft,
    setEditingDraft,
    saving,
    editorErrors,
    deleteTarget,
    deleting,
    runTarget,
    editingSchedule,
    setEditingSchedule,
    scheduleSaving,
    refreshRunbooks,
    startRun,
    cancelRun,
    confirmRun,
    startCreate,
    startEdit,
    cancelEdit,
    saveDraft,
    confirmDelete,
    cancelDelete,
    executeDelete,
    deleteJob,
    approveJob,
    rejectJob,
    saveSchedule,
    deleteSchedule,
    toggleScheduleEnabled,
    triggerSchedule,
    selectRunbook,
  } = useRunbooksPage(search.job)
  const appliedDeepLinkRef = useRef('')
  const forceReconnectOpsEvents = useOpsEventsReconnect()
  const resyncPage = useCallback(() => {
    forceReconnectOpsEvents()
    void refreshRunbooks()
  }, [forceReconnectOpsEvents, refreshRunbooks])

  // Picking a runbook from the mobile drawer should reveal the detail, so close
  // the drawer on select (matches the services navigation behavior).
  const handleSelectRunbook = useCallback(
    (id: string | null) => {
      selectRunbook(id)
      layout.setSidebarOpen(false)
      appliedDeepLinkRef.current = ''
      void navigate({
        search: id == null ? {} : runbookDefinitionSearch(id),
      })
    },
    [layout, navigate, selectRunbook],
  )

  useEffect(() => {
    if (runbooksLoading || focusedJobLoading) return

    if (search.job != null && focusedJobError != null) {
      const key = `job-error:${search.job}`
      if (appliedDeepLinkRef.current === key) return
      appliedDeepLinkRef.current = key
      selectRunbook(null)
      layout.setSidebarOpen(false)
      return
    }

    const resolution = resolveRunbooksDeepLink(search, runbooks, focusedJob)

    if (resolution.kind === 'execution') {
      if (appliedDeepLinkRef.current !== resolution.key) {
        appliedDeepLinkRef.current = resolution.key
        selectRunbook(resolution.selectedRunbookId)
        layout.setSidebarOpen(false)
      }
      if (search.runbook != null) {
        void navigate({
          search: runbookExecutionSearch(resolution.jobId),
          replace: true,
        })
      }
      return
    }

    if (resolution.kind === 'definition') {
      if (appliedDeepLinkRef.current === resolution.key) return
      appliedDeepLinkRef.current = resolution.key
      selectRunbook(resolution.selectedRunbookId)
      layout.setSidebarOpen(false)
      return
    }

    if (resolution.kind === 'invalid') {
      if (search.job != null) {
        const key = `job-error:${search.job}`
        if (appliedDeepLinkRef.current === key) return
        appliedDeepLinkRef.current = key
        selectRunbook(null)
        layout.setSidebarOpen(false)
        return
      }
      appliedDeepLinkRef.current = ''
      void navigate({ search: {}, replace: true })
      return
    }

    if (appliedDeepLinkRef.current !== '') {
      appliedDeepLinkRef.current = ''
      selectRunbook(null)
    }
  }, [
    focusedJobError,
    focusedJob,
    focusedJobLoading,
    layout,
    navigate,
    runbooks,
    runbooksLoading,
    search.job,
    search.runbook,
    search,
    selectRunbook,
  ])

  const showEditor = editingDraft != null
  const showDetail = !showEditor && selectedRunbook != null
  const showStandaloneReceipt =
    !showEditor &&
    search.job != null &&
    focusedJob != null &&
    focusedJobError == null &&
    selectedRunbook == null

  const handleFocusJob = useCallback(
    (jobId: string | null) => {
      appliedDeepLinkRef.current = ''
      void navigate({
        search:
          jobId != null
            ? runbookExecutionSearch(jobId)
            : selectedRunbook != null
              ? runbookDefinitionSearch(selectedRunbook.id)
              : {},
      })
    },
    [navigate, selectedRunbook],
  )

  const handleStartCreate = useCallback(() => {
    appliedDeepLinkRef.current = ''
    void navigate({ search: {} })
    startCreate()
  }, [navigate, startCreate])

  const handleConfirmRun = useCallback(
    async (parameters: Record<string, string>) => {
      const job = await confirmRun(parameters)
      if (job == null) return
      appliedDeepLinkRef.current = ''
      await navigate({
        search: runbookExecutionSearch(job.id),
      })
    },
    [confirmRun, navigate],
  )

  return (
    <AppShell
      sidebar={
        <RunbooksSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          loading={runbooksLoading}
          runbooks={runbooks}
          jobs={jobs}
          schedules={schedules}
          selectedRunbookId={selectedRunbookId}
          onSelectRunbook={handleSelectRunbook}
          onCreateRunbook={handleStartCreate}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[44px_1fr_28px] bg-background">
        <header className="flex min-w-0 items-center justify-between gap-2 border-b border-border bg-card px-2.5">
          <div className="flex min-w-0 items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={() => layout.setSidebarOpen((prev) => !prev)}
              aria-label="Open menu"
            >
              <Menu className="h-5 w-5" />
            </Button>
            <AppSectionTitle hostname={hostname} section="runbooks" />
          </div>
          <ConnectionBadge state={connectionState} onClick={resyncPage} />
        </header>

        <div className="min-h-0 overflow-y-auto p-3 md:overflow-hidden">
          {showEditor && (
            <RunbookEditor
              draft={editingDraft}
              saving={saving}
              errors={editorErrors}
              services={targetServices}
              servicesLoading={targetServicesLoading}
              onDraftChange={setEditingDraft}
              onSave={() => void saveDraft()}
              onCancel={cancelEdit}
            />
          )}

          {!showEditor && (
            <div className="flex flex-col gap-3 md:grid md:h-full md:min-h-0 md:grid-rows-[auto_1fr] md:overflow-hidden">
              <RunbookOperationsSummary
                runbooks={runbooks}
                jobs={jobs}
                schedules={schedules}
                selectedRunbookId={selectedRunbookId}
                onSelectRunbook={handleSelectRunbook}
              />

              {showDetail ? (
                <div className="flex flex-col gap-3 md:grid md:h-full md:min-h-0 md:grid-rows-[auto_1fr] md:overflow-hidden">
                  <RunbookDetailPanel
                    runbook={selectedRunbook}
                    lastJob={selectedJobs[0] ?? null}
                    schedule={selectedSchedule}
                    editingSchedule={editingSchedule}
                    scheduleSaving={scheduleSaving}
                    onEdit={startEdit}
                    onDelete={confirmDelete}
                    onRun={startRun}
                    onEditSchedule={setEditingSchedule}
                    onCancelScheduleEdit={() => setEditingSchedule(null)}
                    onSaveSchedule={(draft) => void saveSchedule(draft)}
                    onDeleteSchedule={(id) => void deleteSchedule(id)}
                    onToggleScheduleEnabled={(s) => void toggleScheduleEnabled(s)}
                    onTriggerSchedule={(id) => void triggerSchedule(id)}
                  />
                  <RunbookJobHistory
                    jobs={selectedJobs}
                    focusJobId={search.job}
                    onFocusJob={handleFocusJob}
                    onDeleteJob={deleteJob}
                    onApproveJob={approveJob}
                    onRejectJob={rejectJob}
                  />
                </div>
              ) : showStandaloneReceipt ? (
                <div className="grid min-h-0 content-start gap-3 overflow-y-auto">
                  <RunbookExecutionReceipt job={focusedJob} standalone />
                </div>
              ) : focusedJobLoading ? (
                <div className="flex min-h-[50vh] items-center justify-center md:h-full md:min-h-0">
                  <p className="text-[13px] text-muted-foreground">Loading execution receipt…</p>
                </div>
              ) : focusedJobError != null && search.job != null ? (
                <div className="flex min-h-[50vh] items-center justify-center md:h-full md:min-h-0">
                  <div className="max-w-md text-center">
                    <p className="text-[13px] font-semibold text-destructive-foreground">
                      Execution receipt unavailable
                    </p>
                    <p className="mt-1 text-[11px] text-muted-foreground">
                      {focusedJobError instanceof Error
                        ? focusedJobError.message
                        : 'The requested execution could not be loaded.'}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="flex min-h-[50vh] items-center justify-center md:h-full md:min-h-0">
                  <div className="text-center">
                    <p className="text-[13px] text-muted-foreground">
                      {runbooks.length > 0 ? (
                        <>
                          <span className="md:hidden">Open the menu to pick a runbook</span>
                          <span className="hidden md:inline">
                            Select a runbook from the sidebar
                          </span>
                        </>
                      ) : (
                        'No runbooks yet'
                      )}
                    </p>
                    <div className="mt-3 flex items-center justify-center gap-2">
                      {runbooks.length > 0 && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 cursor-pointer text-[11px] md:hidden"
                          onClick={() => layout.setSidebarOpen(true)}
                        >
                          Open menu
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-7 cursor-pointer text-[11px]"
                        onClick={handleStartCreate}
                      >
                        Create new runbook
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">{runbooks.length} runbooks</span>
        </footer>
      </main>

      <RunbookRunDialog
        open={runTarget != null}
        runbook={runTarget}
        onConfirm={(parameters) => void handleConfirmRun(parameters)}
        onCancel={cancelRun}
      />

      <RunbookDeleteDialog
        open={deleteTarget != null}
        runbookName={deleteTarget?.name ?? ''}
        deleting={deleting}
        onConfirm={() => void executeDelete()}
        onCancel={cancelDelete}
      />
    </AppShell>
  )
}

export const Route = createFileRoute('/runbooks')({
  validateSearch: parseRunbooksSearch,
  component: RunbooksPage,
})
