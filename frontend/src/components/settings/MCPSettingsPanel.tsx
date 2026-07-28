import { Check, Copy } from 'lucide-react'
import { useMemo, useState } from 'react'

import SecretSettingControl from './SecretSettingControl'
import type { SecretIntent } from './SecretSettingControl'
import {
  EnvironmentOwnership,
  SettingsField,
  SettingValueSummary,
  ValidationError,
} from './SettingsField'
import type { SettingsResponse } from '@/api/settings'
import { Button } from '@/components/ui/button'
import { writeClipboardText } from '@/lib/clipboardProvider'
import { cn } from '@/lib/utils'

type SnippetKind = 'codex' | 'claude' | 'json'

type MCPSettingsPanelProps = {
  hostname: string
  settings: SettingsResponse['integrations']['mcp']
  enabled: boolean
  tokenIntent: SecretIntent
  tokenValue: string
  tokenError?: string
  saving: boolean
  onEnabledChange: (enabled: boolean) => void
  onTokenIntentChange: (intent: SecretIntent) => void
  onTokenValueChange: (value: string) => void
}

function formatMCPServerName(hostname: string): string {
  const normalized = hostname
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized === '' ? 'sentinel' : `sentinel-${normalized}`
}

export default function MCPSettingsPanel({
  hostname,
  settings,
  enabled,
  tokenIntent,
  tokenValue,
  tokenError = '',
  saving,
  onEnabledChange,
  onTokenIntentChange,
  onTokenValueChange,
}: MCPSettingsPanelProps) {
  const [copyError, setCopyError] = useState('')
  const [snippetKind, setSnippetKind] = useState<SnippetKind>('codex')
  const [copied, setCopied] = useState('')
  const serverName = useMemo(() => formatMCPServerName(hostname), [hostname])
  const endpoint = useMemo(() => {
    const path = settings.endpoint || '/mcp'
    return new URL(path, window.location.origin).toString()
  }, [settings.endpoint])

  const snippets = useMemo<Record<SnippetKind, string>>(() => {
    const tokenExport = `export SENTINEL_TOKEN='<same value as server.token>'`
    const jsonConfig = JSON.stringify(
      {
        mcpServers: {
          [serverName]: {
            type: 'http',
            url: endpoint,
            headers: {
              Authorization: 'Bearer ${SENTINEL_TOKEN}',
            },
          },
        },
      },
      null,
      2,
    )
    const claudeConfig = JSON.stringify({
      type: 'http',
      url: endpoint,
      headers: { Authorization: 'Bearer ${SENTINEL_TOKEN}' },
    })
    return {
      codex: `${tokenExport}\ncodex mcp add ${serverName} --url ${endpoint} --bearer-token-env-var SENTINEL_TOKEN`,
      claude: `${tokenExport}\nclaude mcp add-json --scope user ${serverName} '${claudeConfig}'`,
      json: jsonConfig,
    }
  }, [endpoint, serverName])

  const copy = async (key: string, value: string) => {
    setCopyError('')
    let copiedSuccessfully = false
    try {
      copiedSuccessfully = await writeClipboardText(value)
    } catch {
      copiedSuccessfully = false
    }
    if (!copiedSuccessfully) {
      setCopied('')
      setCopyError('Clipboard permission was denied. Select and copy the value manually.')
      return
    }
    setCopied(key)
    window.setTimeout(() => setCopied((current) => (current === key ? '' : current)), 1600)
  }

  const replacementReady = tokenIntent === 'replace' && tokenValue.trim() !== ''
  const canEnable = (settings.token.configured && tokenIntent !== 'clear') || replacementReady
  const runtimeStatus = !enabled
    ? 'Disabled'
    : settings.runtimeTokenConfigured
      ? 'Available'
      : 'Pending restart'
  const selectedSnippet = snippets[snippetKind]

  return (
    <div className="grid gap-3">
      <SettingsField
        label="Remote agent access"
        description="Expose the bounded MCP control plane through the shared Sentinel authentication boundary."
        setting={settings.enabled}
      >
        <div className="grid min-w-0 gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span
              className={cn(
                'rounded-full border px-2 py-1 text-[10px] font-medium',
                runtimeStatus === 'Available'
                  ? 'border-ok/45 bg-ok/10 text-ok-foreground'
                  : runtimeStatus === 'Pending restart'
                    ? 'border-warning/45 bg-warning/10 text-warning-foreground'
                    : 'border-border-subtle bg-surface-overlay text-muted-foreground',
              )}
            >
              {runtimeStatus}
            </span>
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              aria-label="Enable MCP"
              disabled={saving || !settings.enabled.editable || (!enabled && !canEnable)}
              onClick={() => onEnabledChange(!enabled)}
              className="inline-flex min-h-11 min-w-24 items-center justify-between gap-3 rounded-md border border-border-subtle bg-surface-overlay px-3 text-[11px] text-secondary-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            >
              <span>Enabled</span>
              <span
                className={cn(
                  'relative h-5 w-9 rounded-full border transition-colors',
                  enabled ? 'border-primary/60 bg-primary/30' : 'border-border bg-background',
                )}
                aria-hidden="true"
              >
                <span
                  className={cn(
                    'absolute top-0.5 size-3.5 rounded-full bg-muted-foreground transition-transform',
                    enabled && 'translate-x-4 bg-primary',
                  )}
                />
              </span>
            </button>
          </div>

          {!canEnable && (
            <div className="rounded-md border border-warning/45 bg-warning/10 px-3 py-2 text-[11px] leading-relaxed text-warning-foreground">
              Replace the shared token before enabling MCP. A new token and MCP enablement become
              available together after restart.
            </div>
          )}
          {enabled && !settings.runtimeTokenConfigured && (
            <p className="text-[10px] leading-relaxed text-warning-foreground">
              The desired state is enabled, but this process started without the saved token.
              Restart Sentinel to activate the endpoint.
            </p>
          )}
          <EnvironmentOwnership setting={settings.enabled} />
          <SettingValueSummary setting={settings.enabled} />

          <div>
            <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
              Endpoint
            </p>
            <div className="flex min-w-0 items-center gap-2 rounded-md border border-border-subtle bg-surface-overlay px-3 py-2">
              <span
                className={cn(
                  'h-2 w-2 shrink-0 rounded-full',
                  runtimeStatus === 'Available' ? 'bg-ok' : 'bg-muted-foreground/50',
                )}
                aria-hidden="true"
              />
              <code className="min-w-0 flex-1 overflow-x-auto font-mono text-[11px] whitespace-nowrap text-secondary-foreground">
                {endpoint}
              </code>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="min-h-11 shrink-0 gap-1.5 px-2 text-[11px]"
                onClick={() => void copy('endpoint', endpoint)}
              >
                {copied === 'endpoint' ? (
                  <Check className="h-3 w-3" />
                ) : (
                  <Copy className="h-3 w-3" />
                )}
                {copied === 'endpoint' ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
          <ValidationError message={copyError} />
        </div>
      </SettingsField>

      <SettingsField
        label="Shared token"
        description="Manage the same server.token used by browser authentication and MCP clients. It is write-only and restart-based."
        setting={settings.token}
      >
        <SecretSettingControl
          id="settings-mcp-token"
          label="Shared token"
          setting={settings.token}
          intent={tokenIntent}
          value={tokenValue}
          error={tokenError}
          placeholder="Enter a new shared token"
          onIntentChange={onTokenIntentChange}
          onValueChange={onTokenValueChange}
        />
      </SettingsField>

      <SettingsField
        label="Connect an MCP client"
        description="Keep the shared token in an environment variable instead of project files."
      >
        <div className="grid min-w-0 gap-3">
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
            <div className="flex rounded-md border border-border-subtle bg-surface-overlay p-0.5">
              {(['codex', 'claude', 'json'] as const).map((kind) => (
                <button
                  key={kind}
                  type="button"
                  className={cn(
                    'min-h-11 rounded px-2 text-[10px] font-medium transition-colors',
                    snippetKind === kind
                      ? 'bg-primary/15 text-primary-text'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                  onClick={() => {
                    setSnippetKind(kind)
                    setCopied('')
                  }}
                >
                  {kind === 'json' ? 'mcpServers' : kind === 'codex' ? 'Codex' : 'Claude'}
                </button>
              ))}
            </div>
          </div>

          <div className="relative min-w-0 max-w-full">
            <pre className="max-h-64 w-full min-w-0 max-w-full overflow-auto rounded-md border border-border-subtle bg-background p-3 pr-20 font-mono text-[11px] leading-relaxed text-secondary-foreground">
              <code>{selectedSnippet}</code>
            </pre>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="absolute top-2 right-2 min-h-11 gap-1.5 bg-surface-overlay px-2 text-[11px]"
              onClick={() => void copy('snippet', selectedSnippet)}
            >
              {copied === 'snippet' ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
              {copied === 'snippet' ? 'Copied' : 'Copy'}
            </Button>
          </div>
        </div>
      </SettingsField>
    </div>
  )
}
