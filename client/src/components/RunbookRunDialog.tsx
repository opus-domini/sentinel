import { useCallback, useEffect, useState } from 'react'
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
  runbook: OpsRunbook | null
  onConfirm: (parameters: Record<string, string>) => void
  onCancel: () => void
}

function buildDefaults(
  params: Array<RunbookParameter>,
): Record<string, string> {
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

export function RunbookRunDialog({
  open,
  runbook,
  onConfirm,
  onCancel,
}: RunbookRunDialogProps) {
  const params = runbook?.parameters ?? []
  const [values, setValues] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})

  useEffect(() => {
    if (open && params.length > 0) {
      setValues(buildDefaults(params))
      setErrors({})
    }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps -- reset on open

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
            {params.length > 0
              ? 'Configure parameters before running this runbook.'
              : 'This runbook has no parameters. Click Run to execute.'}
          </DialogDescription>
        </DialogHeader>

        {params.length > 0 && (
          <div className="grid gap-3 py-1">
            {params.map((p) => (
              <div key={p.name}>
                <label className="text-[10px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
                  {p.label || p.name}
                  {p.required && (
                    <span className="ml-0.5 text-destructive-foreground">
                      *
                    </span>
                  )}
                </label>

                {p.type === 'boolean' ? (
                  <select
                    value={values[p.name] ?? ''}
                    onChange={(e) => setValue(p.name, e.target.value)}
                    className={cn(
                      'mt-0.5 h-8 w-full rounded-md border border-border-subtle bg-surface-overlay px-2 text-[12px]',
                      errors[p.name] && 'border-destructive',
                    )}
                  >
                    <option value="false">false</option>
                    <option value="true">true</option>
                  </select>
                ) : p.type === 'select' && p.options && p.options.length > 0 ? (
                  <Select
                    value={values[p.name] ?? ''}
                    onValueChange={(v) => setValue(p.name, v)}
                  >
                    <SelectTrigger
                      className={cn(
                        'mt-0.5 h-8 w-full bg-surface-overlay text-[12px]',
                        errors[p.name] && 'border-destructive',
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
                    className={cn(
                      'mt-0.5 h-8 bg-surface-overlay text-[12px]',
                      errors[p.name] && 'border-destructive',
                    )}
                    type={p.type === 'number' ? 'text' : 'text'}
                    inputMode={p.type === 'number' ? 'numeric' : undefined}
                    placeholder={p.default || ''}
                    value={values[p.name] ?? ''}
                    onChange={(e) => setValue(p.name, e.target.value)}
                  />
                )}

                {errors[p.name] && (
                  <p className="mt-0.5 text-[10px] text-destructive-foreground">
                    {errors[p.name]}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={onCancel}
          >
            Cancel
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer gap-1"
            onClick={handleSubmit}
          >
            <Play className="h-3 w-3" />
            Run
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
