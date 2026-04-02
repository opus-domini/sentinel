import { useState } from 'react'
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

export type RunbookStepDraft = {
  key: string
  type: 'run' | 'script' | 'approval'
  title: string
  command: string
  script: string
  description: string
  continueOnError: boolean
  timeout: string
  retries: string
  retryDelay: string
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
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const retriesNum = Number(step.retries) || 0

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
          <option value="run">run</option>
          <option value="script">script</option>
          <option value="approval">approval</option>
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
            className="h-6 w-6 cursor-pointer text-destructive-foreground hover:text-destructive-foreground"
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
              errors.title && 'border-destructive',
            )}
            placeholder="Step title"
            value={step.title}
            onChange={(e) => onChange({ ...step, title: e.target.value })}
          />
          {errors.title && (
            <p className="mt-0.5 text-[10px] text-destructive-foreground">
              {errors.title}
            </p>
          )}
        </div>

        {step.type === 'run' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Command
            </label>
            <Input
              className={cn(
                'mt-0.5 h-7 bg-surface-overlay font-mono text-[11px]',
                errors.command && 'border-destructive',
              )}
              placeholder="systemctl restart nginx"
              value={step.command}
              onChange={(e) => onChange({ ...step, command: e.target.value })}
            />
            {errors.command && (
              <p className="mt-0.5 text-[10px] text-destructive-foreground">
                {errors.command}
              </p>
            )}
          </div>
        )}

        {step.type === 'script' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Script
            </label>
            <Textarea
              className={cn(
                'mt-0.5 min-h-36 bg-surface-overlay font-mono text-sm leading-relaxed',
                errors.script && 'border-destructive',
              )}
              placeholder={
                '#!/bin/bash\nset -euo pipefail\n\n# your script here'
              }
              value={step.script}
              onChange={(e) => onChange({ ...step, script: e.target.value })}
            />
            {errors.script && (
              <p className="mt-0.5 text-[10px] text-destructive-foreground">
                {errors.script}
              </p>
            )}
          </div>
        )}

        {step.type === 'approval' && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground">
              Description
            </label>
            <Textarea
              className="mt-0.5 min-h-12 bg-surface-overlay text-[12px]"
              placeholder="Describe what the operator should verify before approving..."
              value={step.description}
              onChange={(e) =>
                onChange({ ...step, description: e.target.value })
              }
            />
          </div>
        )}

        {/* Advanced options */}
        <button
          type="button"
          className="flex cursor-pointer items-center gap-1 text-[10px] font-medium text-muted-foreground hover:text-foreground"
          onClick={() => setAdvancedOpen(!advancedOpen)}
        >
          {advancedOpen ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
          Advanced
        </button>

        {advancedOpen && (
          <div className="grid gap-2 rounded border border-border-subtle bg-surface-overlay p-2">
            <label className="flex cursor-pointer items-center gap-2 text-[11px] select-none">
              <input
                type="checkbox"
                checked={step.continueOnError}
                onChange={(e) =>
                  onChange({ ...step, continueOnError: e.target.checked })
                }
                className="h-3.5 w-3.5 rounded border-border accent-primary"
              />
              <span className="text-muted-foreground">Continue on error</span>
            </label>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] font-medium text-muted-foreground">
                  Timeout (seconds)
                </label>
                <Input
                  className="mt-0.5 h-7 bg-surface-elevated text-[11px]"
                  type="text"
                  inputMode="numeric"
                  placeholder="30"
                  value={step.timeout}
                  onChange={(e) =>
                    onChange({ ...step, timeout: e.target.value })
                  }
                />
              </div>
              <div>
                <label className="text-[10px] font-medium text-muted-foreground">
                  Retries
                </label>
                <Input
                  className="mt-0.5 h-7 bg-surface-elevated text-[11px]"
                  type="text"
                  inputMode="numeric"
                  placeholder="0"
                  value={step.retries}
                  onChange={(e) =>
                    onChange({ ...step, retries: e.target.value })
                  }
                />
              </div>
            </div>
            {retriesNum > 0 && (
              <div>
                <label className="text-[10px] font-medium text-muted-foreground">
                  Retry delay (seconds)
                </label>
                <Input
                  className="mt-0.5 h-7 bg-surface-elevated text-[11px]"
                  type="text"
                  inputMode="numeric"
                  placeholder="2"
                  value={step.retryDelay}
                  onChange={(e) =>
                    onChange({ ...step, retryDelay: e.target.value })
                  }
                />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
