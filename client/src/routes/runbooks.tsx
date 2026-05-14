import { useCallback } from 'react'
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
import { useTokenContext } from '@/contexts/TokenContext'
import { useOpsEventsReconnect } from '@/hooks/useOpsEvents'
import { useRunbooksPage } from '@/hooks/useRunbooksPage'

function RunbooksPage() {
  const { tokenRequired, hostname } = useMetaContext()
  const { authenticated, setToken } = useTokenContext()
  const layout = useLayoutContext()

  const {
    runbooks,
    jobs,
    schedules,
    runbooksLoading,
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
  } = useRunbooksPage()
  const forceReconnectOpsEvents = useOpsEventsReconnect()
  const resyncPage = useCallback(() => {
    forceReconnectOpsEvents()
    void refreshRunbooks()
  }, [forceReconnectOpsEvents, refreshRunbooks])

  const showEditor = editingDraft != null
  const showDetail = !showEditor && selectedRunbook != null

  return (
    <AppShell
      sidebar={
        <RunbooksSidebar
          isOpen={layout.sidebarOpen}
          collapsed={layout.sidebarCollapsed}
          tokenRequired={tokenRequired}
          authenticated={authenticated}
          loading={runbooksLoading}
          runbooks={runbooks}
          jobs={jobs}
          schedules={schedules}
          selectedRunbookId={selectedRunbookId}
          onTokenChange={setToken}
          onSelectRunbook={selectRunbook}
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

        <div className="min-h-0 overflow-hidden p-3">
          {showEditor && (
            <RunbookEditor
              draft={editingDraft}
              saving={saving}
              errors={editorErrors}
              onDraftChange={setEditingDraft}
              onSave={() => void saveDraft()}
              onCancel={cancelEdit}
            />
          )}

          {!showEditor && (
            <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden">
              <RunbookOperationsSummary
                runbooks={runbooks}
                jobs={jobs}
                schedules={schedules}
                selectedRunbookId={selectedRunbookId}
                onSelectRunbook={selectRunbook}
              />

              {showDetail ? (
                <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden">
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
                    onToggleScheduleEnabled={(s) =>
                      void toggleScheduleEnabled(s)
                    }
                    onTriggerSchedule={(id) => void triggerSchedule(id)}
                  />
                  <RunbookJobHistory
                    jobs={selectedJobs}
                    onDeleteJob={deleteJob}
                    onApproveJob={approveJob}
                    onRejectJob={rejectJob}
                  />
                </div>
              ) : (
                <div className="flex h-full items-center justify-center">
                  <div className="text-center">
                    <p className="text-[13px] text-muted-foreground">
                      {runbooks.length > 0
                        ? 'Select a runbook from the sidebar'
                        : 'No runbooks yet'}
                    </p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="mt-3 h-7 cursor-pointer text-[11px]"
                      onClick={startCreate}
                    >
                      Create new runbook
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <footer className="flex items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2.5 text-[12px] text-secondary-foreground">
          <span className="min-w-0 flex-1 truncate">
            {runbooks.length} runbooks
          </span>
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
  component: RunbooksPage,
})
