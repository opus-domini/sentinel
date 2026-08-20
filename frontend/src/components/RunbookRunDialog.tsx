import { useCallback, useId, useMemo, useState } from 'react'
import { Play } from 'lucide-react'
import type { OpsRunbook, RunbookParameter } from '@/types'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { cn } from '@/lib/utils'

type RunbookRunDialogProps = {
  open: boolean
  runbook:
    | (Pick<OpsRunbook, 'id' | 'name' | 'steps' | 'parameters' | 'targetService'> & {
        description?: string
      })
    | null
  confirming?: boolean
  onConfirm: (parameters: Record<string, string>) => void
  onCancel: () => void
}

function buildDefaults(params: Array<RunbookParameter>): Record<string, string> {
  const values: Record<string, string> = {}
  for (const p of params) {
    values[p.name] = p.default ?? ''
  }
  return values
}

function validateParams(
  params: Array<RunbookParameter>,
  values: Record<string, string>,
): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const p of params) {
    const val = (values[p.name] ?? '').trim()
    if (p.required && val === '') {
      errors[p.name] = `${p.label || p.name} is required`
    }
    if (p.type === 'number' && val !== '' && Number.isNaN(Number(val))) {
      errors[p.name] = 'Must be a number'
    }
  }
  return errors
}

function stepsWithStableKeys(steps: OpsRunbook['steps']) {
  const occurrences = new Map<string, number>()
  return steps.map((step) => {
    const signature = JSON.stringify(step)
    const occurrence = occurrences.get(signature) ?? 0
    occurrences.set(signature, occurrence + 1)
    return { key: `${signature}:${occurrence}`, step }
  })
}

export function RunbookRunDialog({
  open,
  runbook,
  confirming = false,
  onConfirm,
  onCancel,
}: RunbookRunDialogProps) {
  const id = useId()
  const params = useMemo(() => runbook?.parameters ?? [], [runbook])
  const steps = useMemo(() => stepsWithStableKeys(runbook?.steps ?? []), [runbook])
  const [values, setValues] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const formKey = open && runbook ? `${runbook.id}:${JSON.stringify(params)}` : ''
  const [activeFormKey, setActiveFormKey] = useState('')

  if (formKey !== activeFormKey) {
    setActiveFormKey(formKey)
    if (formKey !== '') {
      setValues(buildDefaults(params))
      setErrors({})
    }
  }

  const setValue = useCallback((name: string, value: string) => {
    setValues((prev) => ({ ...prev, [name]: value }))
    setErrors((prev) => {
      if (!prev[name]) return prev
      const next = { ...prev }
      delete next[name]
      return next
    })
  }, [])

  const handleSubmit = useCallback(() => {
    const errs = validateParams(params, values)
    if (Object.keys(errs).length > 0) {
      setErrors(errs)
      return
    }
    onConfirm(values)
  }, [params, values, onConfirm])

  if (!runbook) return null

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onCancel()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Run {runbook.name}</DialogTitle>
          <DialogDescription>
            Review the target and execution steps before starting this runbook.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 rounded-md border border-border-subtle bg-surface-raised p-3 text-[11px]">
          {runbook.description && (
            <p className="leading-relaxed text-secondary-foreground">{runbook.description}</p>
          )}
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Target
            </p>
            <p className="mt-0.5 font-medium">
              {runbook.targetService ? `Service · ${runbook.targetService}` : 'No service target'}
            </p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
              Steps and effects
            </p>
            <ol className="mt-1 grid gap-1">
              {steps.map(({ key, step }, index) => (
                <li key={key} className="flex items-start gap-2">
                  <span className="text-muted-foreground">{index + 1}.</span>
                  <span>
                    {step.title}
                    <span className="ml-1 text-[10px] uppercase text-muted-foreground">
                      {step.type === 'approval' ? 'approval required' : step.type}
                    </span>
                  </span>
                </li>
              ))}
            </ol>
          </div>
        </div>

        {params.length > 0 && (
          <div className="grid gap-3 py-1">
            {params.map((p, index) => {
              const fieldId = `${id}-param-${index}`
              const errorId = `${fieldId}-error`
              const error = errors[p.name]

              return (
                <div key={p.name}>
                  <label
                    htmlFor={fieldId}
                    className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground"
                  >
                    {p.label || p.name}
                    {p.required && <span className="ml-0.5 text-destructive-foreground">*</span>}
                  </label>

                  {p.type === 'boolean' ? (
                    <select
                      id={fieldId}
                      value={values[p.name] ?? ''}
                      aria-invalid={error ? true : undefined}
                      aria-describedby={error ? errorId : undefined}
                      onChange={(e) => setValue(p.name, e.target.value)}
                      className={cn(
                        'mt-0.5 h-8 w-full rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]',
                        error && 'border-destructive',
                      )}
                    >
                      <option value="false">false</option>
                      <option value="true">true</option>
                    </select>
                  ) : p.type === 'select' && p.options && p.options.length > 0 ? (
                    <Select value={values[p.name] ?? ''} onValueChange={(v) => setValue(p.name, v)}>
                      <SelectTrigger
                        id={fieldId}
                        aria-invalid={error ? true : undefined}
                        aria-describedby={error ? errorId : undefined}
                        className={cn(
                          'mt-0.5 h-8 w-full bg-surface-overlay text-[12px]',
                          error && 'border-destructive',
                        )}
                      >
                        <SelectValue placeholder="Select..." />
                      </SelectTrigger>
                      <SelectContent>
                        {p.options.map((opt) => (
                          <SelectItem key={opt} value={opt}>
                            {opt}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      id={fieldId}
                      className={cn(
                        'mt-0.5 h-8 bg-surface-overlay text-[12px]',
                        error && 'border-destructive',
                      )}
                      type={p.type === 'number' ? 'text' : 'text'}
                      inputMode={p.type === 'number' ? 'numeric' : undefined}
                      placeholder={p.default || ''}
                      value={values[p.name] ?? ''}
                      aria-invalid={error ? true : undefined}
                      aria-describedby={error ? errorId : undefined}
                      onChange={(e) => setValue(p.name, e.target.value)}
                    />
                  )}

                  {error && (
                    <p
                      id={errorId}
                      role="alert"
                      className="mt-0.5 text-[10px] text-destructive-foreground"
                    >
                      {error}
                    </p>
                  )}
                </div>
              )
            })}
          </div>
        )}

        <p className="text-[10px] leading-relaxed text-muted-foreground">
          Parameter values are persisted in the execution receipt and remain visible to operators.
        </p>

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            disabled={confirming}
            onClick={onCancel}
          >
            Cancel
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer gap-1"
            disabled={confirming}
            onClick={handleSubmit}
          >
            <Play className="h-3 w-3" />
            {confirming ? 'Starting...' : 'Run'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
