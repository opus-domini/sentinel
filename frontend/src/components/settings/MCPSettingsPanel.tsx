import { Check, Copy } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import { Button } from '@/components/ui/button'
import { useTmuxApi } from '@/hooks/useTmuxApi'
import { writeClipboardText } from '@/lib/clipboardProvider'
import { cn } from '@/lib/utils'

type MCPSettings = {
  enabled: boolean
  tokenConfigured: boolean
  endpoint: string
}

type SnippetKind = 'codex' | 'claude' | 'json'

type MCPSettingsPanelProps = {
  hostname: string
}

const MCP_SETTINGS_QUERY_KEY = ['settings', 'mcp'] as const

function formatMCPServerName(hostname: string): string {
  const normalized = hostname
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized === '' ? 'sentinel' : `sentinel-${normalized}`
}

export default function MCPSettingsPanel({ hostname }: MCPSettingsPanelProps) {
  const api = useTmuxApi()
  const queryClient = useQueryClient()
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [snippetKind, setSnippetKind] = useState<SnippetKind>('codex')
  const [copied, setCopied] = useState('')

  const settingsQuery = useQuery({
    queryKey: MCP_SETTINGS_QUERY_KEY,
    queryFn: () => api<MCPSettings>('/api/ops/settings/mcp'),
  })

  const settings = settingsQuery.data
  const serverName = useMemo(() => formatMCPServerName(hostname), [hostname])
  const endpoint = useMemo(() => {
    const path = settings?.endpoint || '/mcp'
    return new URL(path, window.location.origin).toString()
  }, [settings?.endpoint])

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

  const copy = (key: string, value: string) => {
    writeClipboardText(value)
    setCopied(key)
    window.setTimeout(() => setCopied((current) => (current === key ? '' : current)), 1600)
  }

  const setEnabled = async (enabled: boolean) => {
    setSaving(true)
    setSaveError('')
    try {
      const next = await api<MCPSettings>('/api/ops/settings/mcp', {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      })
      queryClient.setQueryData(MCP_SETTINGS_QUERY_KEY, next)
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Failed to update MCP')
    } finally {
      setSaving(false)
    }
  }

  if (settingsQuery.isLoading) {
    return <p className="text-xs text-muted-foreground">Loading MCP settings…</p>
  }

  const loadError =
    settingsQuery.error instanceof Error
      ? settingsQuery.error.message
      : 'Failed to load MCP settings'
  if (!settings) {
    return <p className="text-xs text-destructive-foreground">{loadError}</p>
  }

  const selectedSnippet = snippets[snippetKind]

  return (
    <div className="grid min-w-0 gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-xs font-medium">Remote agent access</h3>
            <span
              className={cn(
                'rounded-full border px-2 py-0.5 text-[10px] font-medium',
                settings.enabled
                  ? 'border-ok/45 bg-ok/10 text-ok-foreground'
                  : 'border-border-subtle bg-surface-overlay text-muted-foreground',
              )}
            >
              {settings.enabled ? 'Available' : 'Disabled'}
            </span>
          </div>
          <p className="mt-1 max-w-lg text-xs leading-relaxed text-muted-foreground">
            Let MCP clients list and create tmux sessions, inspect windows and panes, and interact
            with a live terminal.
          </p>
        </div>
        <label className="flex cursor-pointer items-center gap-2 text-[12px] select-none">
          <input
            type="checkbox"
            aria-label="Enable MCP"
            checked={settings.enabled}
            disabled={saving || !settings.tokenConfigured}
            onChange={(event) => void setEnabled(event.target.checked)}
            className="h-3.5 w-3.5 rounded border-border accent-primary"
          />
          <span className="text-muted-foreground">{saving ? 'Saving…' : 'Enabled'}</span>
        </label>
      </div>

      {!settings.tokenConfigured && (
        <div className="rounded-md border border-warning/45 bg-warning/10 px-3 py-2 text-[11px] leading-relaxed text-warning-foreground">
          Configure <code className="font-mono">server.token</code> before enabling MCP. The
          endpoint uses that same value as its Bearer token.
        </div>
      )}
      {saveError !== '' && (
        <div className="rounded-md border border-destructive/45 bg-destructive/10 px-3 py-2 text-[11px] text-destructive-foreground">
          {saveError}
        </div>
      )}

      <div>
        <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
          Endpoint
        </p>
        <div className="flex min-w-0 items-center gap-2 rounded-md border border-border-subtle bg-surface-overlay px-3 py-2">
          <span
            className={cn(
              'h-2 w-2 shrink-0 rounded-full',
              settings.enabled ? 'bg-ok' : 'bg-muted-foreground/50',
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
            className="h-7 shrink-0 gap-1.5 px-2 text-[11px]"
            onClick={() => copy('endpoint', endpoint)}
          >
            {copied === 'endpoint' ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            {copied === 'endpoint' ? 'Copied' : 'Copy'}
          </Button>
        </div>
      </div>

      <div className="min-w-0 border-t border-border-subtle pt-4">
        <div className="mb-2 flex min-w-0 flex-wrap items-center justify-between gap-2">
          <div>
            <h3 className="text-xs font-medium">Connect an MCP client</h3>
            <p className="mt-0.5 text-[11px] text-muted-foreground">
              Keep the token in an environment variable instead of storing it in project files.
            </p>
          </div>
          <div className="flex rounded-md border border-border-subtle bg-surface-overlay p-0.5">
            {(['codex', 'claude', 'json'] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                className={cn(
                  'rounded px-2 py-1 text-[10px] font-medium transition-colors',
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
            className="absolute top-2 right-2 h-7 gap-1.5 bg-surface-overlay px-2 text-[11px]"
            onClick={() => copy('snippet', selectedSnippet)}
          >
            {copied === 'snippet' ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            {copied === 'snippet' ? 'Copied' : 'Copy'}
          </Button>
        </div>
      </div>
    </div>
  )
}
