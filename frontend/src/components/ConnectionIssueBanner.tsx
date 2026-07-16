import { Check, Copy, ShieldAlert } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { ConnectionIssue } from '@/contexts/ConnectionHealthContext'
import { writeClipboardText } from '@/lib/clipboardProvider'
import { Button } from '@/components/ui/button'

export function ConnectionIssueBanner({
  issue,
  checking,
  onRetry,
}: {
  issue: ConnectionIssue
  checking: boolean
  onRetry: () => void
}) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setCopied(false)
  }, [issue])

  const instruction = issue.configPath
    ? `Edit ${issue.configPath}, restart Sentinel, then retry.`
    : 'Correct the server configuration, restart Sentinel, then retry.'

  return (
    <div
      role="alert"
      aria-live="assertive"
      aria-labelledby="connection-issue-title"
      className="fixed inset-0 z-50 grid place-items-center bg-background/85 p-4"
    >
      <div className="w-full max-w-xl rounded-lg border border-destructive/45 bg-background p-5 shadow-lg">
        <div className="flex items-start gap-3">
          <ShieldAlert className="mt-0.5 h-6 w-6 shrink-0 text-destructive" aria-hidden="true" />
          <div className="min-w-0">
            <p className="text-[11px] font-medium tracking-wide text-destructive uppercase">
              {issue.code}
            </p>
            <h2 id="connection-issue-title" className="mt-0.5 text-lg font-semibold">
              {issue.title}
            </h2>
            <p className="mt-2 text-sm leading-relaxed text-secondary-foreground">
              {issue.message}
            </p>
            <p className="mt-2 text-sm leading-relaxed text-secondary-foreground">{instruction}</p>
          </div>
        </div>

        {issue.configuration !== '' && (
          <div className="mt-4">
            <div className="mb-1.5 flex items-center justify-between gap-2">
              <p className="text-xs font-medium text-foreground">Required configuration</p>
              <Button
                type="button"
                variant="ghost"
                className="h-11 gap-2 px-3"
                onClick={() => {
                  writeClipboardText(issue.configuration)
                  setCopied(true)
                }}
              >
                {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
            <pre className="overflow-x-auto rounded-md border border-border-subtle bg-surface-overlay p-3 text-xs leading-relaxed text-foreground">
              <code>{issue.configuration}</code>
            </pre>
          </div>
        )}

        <div className="mt-5 flex justify-end">
          <Button type="button" className="h-11 px-5" disabled={checking} onClick={onRetry}>
            {checking ? 'Checking…' : 'Retry'}
          </Button>
        </div>
      </div>
    </div>
  )
}
