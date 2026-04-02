import { X } from 'lucide-react'
import type { RunbookParameterType } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export type RunbookParameterDraft = {
  key: string
  name: string
  label: string
  type: RunbookParameterType
  default: string
  required: boolean
  options: string
}

type RunbookParameterEditorProps = {
  param: RunbookParameterDraft
  errors: Record<string, string>
  onChange: (param: RunbookParameterDraft) => void
  onRemove: () => void
}

export function RunbookParameterEditor({
  param,
  errors,
  onChange,
  onRemove,
}: RunbookParameterEditorProps) {
  return (
    <div className="grid gap-2 rounded-lg border border-border-subtle bg-surface-elevated p-2.5">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
          Parameter
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-pointer text-destructive-foreground hover:text-destructive-foreground"
          onClick={onRemove}
          aria-label="Remove parameter"
        >
          <X className="h-3 w-3" />
        </Button>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Name
          </label>
          <Input
            className={cn(
              'mt-0.5 h-7 bg-surface-overlay font-mono text-[11px]',
              errors.name && 'border-destructive',
            )}
            placeholder="ENV"
            value={param.name}
            onChange={(e) => onChange({ ...param, name: e.target.value })}
          />
          {errors.name && (
            <p className="mt-0.5 text-[10px] text-destructive-foreground">
              {errors.name}
            </p>
          )}
        </div>
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Label
          </label>
          <Input
            className="mt-0.5 h-7 bg-surface-overlay text-[12px]"
            placeholder="Environment"
            value={param.label}
            onChange={(e) => onChange({ ...param, label: e.target.value })}
          />
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Type
          </label>
          <select
            value={param.type}
            onChange={(e) =>
              onChange({
                ...param,
                type: e.target.value as RunbookParameterType,
              })
            }
            className="mt-0.5 h-7 w-full rounded-md border border-border-subtle bg-surface-overlay px-1.5 text-[11px]"
          >
            <option value="string">string</option>
            <option value="number">number</option>
            <option value="boolean">boolean</option>
            <option value="select">select</option>
          </select>
        </div>
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Default
          </label>
          <Input
            className="mt-0.5 h-7 bg-surface-overlay text-[12px]"
            placeholder={param.type === 'boolean' ? 'false' : ''}
            value={param.default}
            onChange={(e) => onChange({ ...param, default: e.target.value })}
          />
        </div>
        <div className="flex items-end pb-1">
          <label className="flex cursor-pointer items-center gap-1.5 text-[11px] select-none">
            <input
              type="checkbox"
              checked={param.required}
              onChange={(e) =>
                onChange({ ...param, required: e.target.checked })
              }
              className="h-3.5 w-3.5 rounded border-border accent-primary"
            />
            <span className="text-muted-foreground">Required</span>
          </label>
        </div>
      </div>

      {param.type === 'select' && (
        <div>
          <label className="text-[10px] font-medium text-muted-foreground">
            Options (comma-separated)
          </label>
          <Input
            className={cn(
              'mt-0.5 h-7 bg-surface-overlay text-[12px]',
              errors.options && 'border-destructive',
            )}
            placeholder="production, staging, development"
            value={param.options}
            onChange={(e) => onChange({ ...param, options: e.target.value })}
          />
          {errors.options && (
            <p className="mt-0.5 text-[10px] text-destructive-foreground">
              {errors.options}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
