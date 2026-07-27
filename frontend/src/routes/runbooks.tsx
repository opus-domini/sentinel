import { useCallback, useEffect, useRef } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import AppSectionTitle from '@/components/layout/AppSectionTitle'
import AppShell from '@/components/layout/AppShell'
import ConnectionBadge from '@/components/ConnectionBadge'
import { RunbookDeleteDialog } from '@/components/RunbookDeleteDialog'
import { RunbookEditor } from '@/components/RunbookEditor'
import { RunbookRunDialog } from '@/components/RunbookRunDialog'
import { RunbookDetailPanel } from '@/components/runbooks/RunbookDetailPanel'
import { RunbookJobHistory } from '@/components/runbooks/RunbookJobHistory'
import { RunbookOperationsSummary } from '@/components/runbooks/RunbookOperationsSummary'
import RunbooksSidebar from '@/components/RunbooksSidebar'
import { Button } from '@/components/ui/button'
import { useLayoutContext } from '@/contexts/LayoutContext'
import { useMetaContext } from '@/contexts/MetaContext'
import { useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useRunbooksPage } from '@/hooks/useRunbooksPage'
import { parseRunbooksSearch } from '@/lib/deepLinks'

function RunbooksPage() {
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
      if (search.runbook) {
        appliedDeepLinkRef.current = ''
        void navigate({ search: {}, replace: true })
      }
    },
    [layout, navigate, search.runbook, selectRunbook],
  )

  useEffect(() => {
    if (runbooksLoading || focusedJobLoading || !search.runbook) return
    const runbook = runbooks.find((item) => item.id === search.runbook)
    const job = search.job ? jobs.find((item) => item.id === search.job) : undefined
    if (
      !runbook ||
      focusedJobError != null ||
      (search.job != null && (job == null || job.runbookId !== runbook.id))
    ) {
      appliedDeepLinkRef.current = ''
      void navigate({ search: {}, replace: true })
      return
    }
    const key = `${runbook.id}:${search.job ?? ''}`
    if (appliedDeepLinkRef.current === key) return
    appliedDeepLinkRef.current = key
    selectRunbook(runbook.id)
    layout.setSidebarOpen(false)
  }, [
    focusedJobError,
    focusedJobLoading,
    jobs,
    layout,
    navigate,
    runbooks,
    runbooksLoading,
    search.job,
    search.runbook,
    selectRunbook,
  ])

  const showEditor = editingDraft != null
  const showDetail = !showEditor && selectedRunbook != null

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
          onCreateRunbook={startCreate}
        />
      }
    >
      <main className="grid h-full min-h-0 min-w-0 grid-cols-1 grid-rows-[40px_1fr_28px] bg-background">
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
          <div className="flex items-center gap-1.5">
            <ConnectionBadge state={connectionState} onClick={resyncPage} />
          </div>
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
                    onDeleteJob={deleteJob}
                    onApproveJob={approveJob}
                    onRejectJob={rejectJob}
                  />
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
                        onClick={startCreate}
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
        onConfirm={confirmRun}
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
