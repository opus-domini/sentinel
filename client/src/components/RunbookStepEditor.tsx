import { ArrowDown, ArrowUp, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

export type RunbookStepDraft = {
  key: string
  type: 'command' | 'check' | 'manual'
  title: string
  command: string
  check: string
  description: string
}

type RunbookStepEditorProps = {
  index: number
  step: RunbookStepDraft
  errors: Record<string, string>
  isFirst: boolean
  isLast: boolean
  onChange: (step: RunbookStepDraft) => void
  onMoveUp: () => void
  onMoveDown: () => void
  onRemove: () => void
}

export function RunbookStepEditor({
  index,
  step,
  errors,
  isFirst,
  isLast,
  onChange,
  onMoveUp,
  onMoveDown,
  onRemove,
}: RunbookStepEditorProps) {
  return (
    <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
      <div className="flex items-center gap-2">
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-surface-overlay text-[10px] font-semibold text-muted-foreground">
          {index + 1}
        </span>
        <select
          value={step.type}
          onChange={(e) =>
            onChange({
              ...step,
              type: e.target.value as RunbookStepDraft['type'],
            })
          }
          className="h-6 rounded-md border border-border-subtle bg-surface-overlay px-1.5 text-[11px]"
        >
          <option value="command">command</option>
          <option value="check">check</option>
          <option value="manual">manual</option>
        </select>
        <div className="ml-auto flex items-center gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer"
            disabled={isFirst}
            onClick={onMoveUp}
            aria-label="Move step up"
          >
            <ArrowUp className="h-3 w-3" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer"
            disabled={isLast}
            onClick={onMoveDown}
            aria-label="Move step down"
          >
            <ArrowDown className="h-3 w-3" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer text-red-400 hover:text-red-300"
            onClick={onRemove}
            aria-label="Remove step"
          >
            <X className="h-3 w-3" />
          </Button>
        </div>
      </div>

      <div className="grid gap-1.5">
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Title
          </label>
          <Input
            className={cn(
              'mt-0.5 h-7 bg-surface-overlay text-[12px]',
              errors.title && 'border-red-500',
            )}
            placeholder="Step title"
            value={step.title}
            onChange={(e) => onChange({ ...step, title: e.target.value })}
          />
          {errors.title && (
            <p className="mt-0.5 text-[10px] text-red-400">{errors.title}</p>
          )}
        </div>

        {step.type === 'command' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Command
            </label>
            <Input
              className={cn(
                'mt-0.5 h-7 bg-surface-overlay font-mono text-[11px]',
                errors.command && 'border-red-500',
              )}
              placeholder="sh -c ..."
              value={step.command}
              onChange={(e) => onChange({ ...step, command: e.target.value })}
            />
            {errors.command && (
              <p className="mt-0.5 text-[10px] text-red-400">
                {errors.command}
              </p>
            )}
          </div>
        )}

        {step.type === 'check' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Check
            </label>
            <Input
              className={cn(
                'mt-0.5 h-7 bg-surface-overlay font-mono text-[11px]',
                errors.check && 'border-red-500',
              )}
              placeholder="curl -f http://localhost:80/health"
              value={step.check}
              onChange={(e) => onChange({ ...step, check: e.target.value })}
            />
            {errors.check && (
              <p className="mt-0.5 text-[10px] text-red-400">{errors.check}</p>
            )}
          </div>
        )}

        {step.type === 'manual' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Description
            </label>
            <Textarea
              className="mt-0.5 min-h-12 bg-surface-overlay text-[12px]"
              placeholder="Describe what to check manually..."
              value={step.description}
              onChange={(e) =>
                onChange({ ...step, description: e.target.value })
              }
            />
          </div>
        )}
      </div>
    </div>
  )
}
