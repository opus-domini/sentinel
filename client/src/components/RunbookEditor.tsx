import { ArrowLeft, Plus, Save } from 'lucide-react'
import type { RunbookStepDraft } from '@/components/RunbookStepEditor'
import { RunbookStepEditor } from '@/components/RunbookStepEditor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { cn, randomId } from '@/lib/utils'

export type RunbookDraft = {
  id: string | null
  name: string
  description: string
  enabled: boolean
  webhookURL: string
  steps: Array<RunbookStepDraft>
}

type RunbookEditorProps = {
  draft: RunbookDraft
  saving: boolean
  errors: Record<string, string>
  onDraftChange: (draft: RunbookDraft) => void
  onSave: () => void
  onCancel: () => void
}

function createBlankStep(): RunbookStepDraft {
  return {
    key: randomId(),
    type: 'command',
    title: '',
    command: '',
    check: '',
    description: '',
  }
}

export { createBlankStep }

export function RunbookEditor({
  draft,
  saving,
  errors,
  onDraftChange,
  onSave,
  onCancel,
}: RunbookEditorProps) {
  const isCreating = draft.id === null

  const addStep = () => {
    onDraftChange({
      ...draft,
      steps: [...draft.steps, createBlankStep()],
    })
  }

  const updateStep = (index: number, step: RunbookStepDraft) => {
    const next = [...draft.steps]
    next[index] = step
    onDraftChange({ ...draft, steps: next })
  }

  const removeStep = (index: number) => {
    onDraftChange({
      ...draft,
      steps: draft.steps.filter((_, i) => i !== index),
    })
  }

  const moveStep = (from: number, to: number) => {
    if (to < 0 || to >= draft.steps.length) return
    const next = [...draft.steps]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onDraftChange({ ...draft, steps: next })
  }

  return (
    <div className="grid h-full min-h-0 grid-rows-[auto_1fr] gap-3 overflow-hidden">
      <div className="flex items-center justify-between gap-2 rounded-lg border border-border-subtle bg-surface-elevated px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 cursor-pointer"
            onClick={onCancel}
            aria-label="Cancel editing"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <span className="text-[13px] font-semibold">
            {isCreating ? 'New Runbook' : 'Edit Runbook'}
          </span>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="h-7 cursor-pointer gap-1 px-3 text-[11px]"
          disabled={saving}
          onClick={onSave}
        >
          <Save className="h-3 w-3" />
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </div>

      <ScrollArea className="h-full min-h-0">
        <div className="grid gap-3 pb-4">
          <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-3">
            <div>
              <label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Name
              </label>
              <Input
                className={cn(
                  'mt-0.5 h-8 bg-surface-overlay text-[12px]',
                  errors.name && 'border-red-500',
                )}
                placeholder="Runbook name"
                value={draft.name}
                onChange={(e) =>
                  onDraftChange({ ...draft, name: e.target.value })
                }
              />
              {errors.name && (
                <p className="mt-0.5 text-[10px] text-red-400">{errors.name}</p>
              )}
            </div>
            <div>
              <label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Description
              </label>
              <Textarea
                className="mt-0.5 min-h-12 bg-surface-overlay text-[12px]"
                placeholder="What does this runbook do?"
                value={draft.description}
                onChange={(e) =>
                  onDraftChange({ ...draft, description: e.target.value })
                }
              />
            </div>
            <label className="flex cursor-pointer items-center gap-2 text-[12px] select-none">
              <input
                type="checkbox"
                checked={draft.enabled}
                onChange={(e) =>
                  onDraftChange({ ...draft, enabled: e.target.checked })
                }
                className="h-3.5 w-3.5 rounded border-border accent-primary"
              />
              <span className="text-muted-foreground">Enabled</span>
            </label>
            <div>
              <label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Webhook URL
              </label>
              <Input
                className={cn(
                  'mt-0.5 h-8 bg-surface-overlay text-[12px]',
                  errors.webhookURL && 'border-red-500',
                )}
                placeholder="https://hooks.example.com/..."
                value={draft.webhookURL}
                onChange={(e) =>
                  onDraftChange({ ...draft, webhookURL: e.target.value })
                }
              />
              {errors.webhookURL && (
                <p className="mt-0.5 text-[10px] text-red-400">
                  {errors.webhookURL}
                </p>
              )}
            </div>
          </div>

          <div className="grid gap-2">
            <div className="flex items-center justify-between px-1">
              <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                Steps ({draft.steps.length})
              </span>
              <Button
                variant="outline"
                size="sm"
                className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
                onClick={addStep}
              >
                <Plus className="h-3 w-3" />
                Add Step
              </Button>
            </div>
            {errors.steps && (
              <p className="px-1 text-[10px] text-red-400">{errors.steps}</p>
            )}
            {draft.steps.map((step, i) => (
              <RunbookStepEditor
                key={step.key}
                index={i}
                step={step}
                errors={
                  Object.fromEntries(
                    Object.entries(errors)
                      .filter(([k]) => k.startsWith(`step.${i}.`))
                      .map(([k, v]) => [k.replace(`step.${i}.`, ''), v]),
                  ) as Record<string, string>
                }
                isFirst={i === 0}
                isLast={i === draft.steps.length - 1}
                onChange={(s) => updateStep(i, s)}
                onMoveUp={() => moveStep(i, i - 1)}
                onMoveDown={() => moveStep(i, i + 1)}
                onRemove={() => removeStep(i)}
              />
            ))}
            {draft.steps.length === 0 && (
              <div className="rounded-lg border border-dashed border-border-subtle p-4 text-center">
                <p className="text-[12px] text-muted-foreground">
                  No steps yet. Click &ldquo;Add Step&rdquo; to get started.
                </p>
              </div>
            )}
          </div>
        </div>
      </ScrollArea>
    </div>
  )
}
