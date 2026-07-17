# MCP Tmux Control

Sentinel can expose its tmux control plane as a Streamable HTTP MCP server at
`/mcp`. This is intended for agents that need to work inside a remote machine's
existing tmux sessions without SSH access.

The server uses the official
[Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk)
and is disabled by default.

## Enable

Configure the existing Sentinel token and enable MCP:

```toml
[server]
token = "strong-secret"

[mcp]
enabled = true
```

The same setting is available in **Settings > MCP** and through
`SENTINEL_MCP_ENABLED=true`. The settings screen shows the effective endpoint
and ready-to-copy client configurations.

Every MCP request must send:

```http
Authorization: Bearer strong-secret
```

The Sentinel login cookie is not an MCP credential. There is no separate MCP
token.

## Connect

For a Sentinel served at `https://sentinel.example.ts.net`:

### Codex

```bash
export SENTINEL_TOKEN='<same value as server.token>'
codex mcp add sentinel \
  --url https://sentinel.example.ts.net/mcp \
  --bearer-token-env-var SENTINEL_TOKEN
```

### Claude Code

Claude Code expands environment variables in HTTP headers stored in its MCP
configuration:

```bash
export SENTINEL_TOKEN='<same value as server.token>'
claude mcp add-json --scope user sentinel \
  '{"type":"http","url":"https://sentinel.example.ts.net/mcp","headers":{"Authorization":"Bearer ${SENTINEL_TOKEN}"}}'
```

### `mcpServers` JSON

```json
{
  "mcpServers": {
    "sentinel": {
      "type": "http",
      "url": "https://sentinel.example.ts.net/mcp",
      "headers": {
        "Authorization": "Bearer ${SENTINEL_TOKEN}"
      }
    }
  }
}
```

Environment-variable expansion in a generic `mcpServers` file depends on the
client. Do not commit the literal token.

## Tools

| Tool | Purpose |
| --- | --- |
| `tmux_list_sessions` | List sessions visible to Sentinel |
| `tmux_create_session` | Create a detached session |
| `tmux_list_windows` | Inspect stable window IDs and metadata |
| `tmux_list_panes` | Inspect pane IDs, commands, paths, and geometry |
| `tmux_attach` | Open a native tmux control-mode attachment and capture the active pane |
| `tmux_interact` | Send ordered literal-text and named-key actions, then wait and capture the pane |
| `tmux_read` | Long-poll incremental control events after a cursor |
| `tmux_detach` | Release the MCP attachment without killing the tmux session |

There is deliberately no raw tmux-command tool.

## Interaction Model

`tmux_attach` returns an `attachmentId`, active `paneId`, event `cursor`, and
the current visible screen. Use the stable pane ID for subsequent calls.

`tmux_interact` accepts an ordered list so text and special keys are explicit:

```json
{
  "attachmentId": "att_...",
  "paneId": "%12",
  "input": [
    { "type": "text", "value": "npm test" },
    { "type": "key", "value": "Enter" }
  ],
  "wait": {
    "mode": "idle",
    "quietMs": 400,
    "timeoutMs": 5000
  }
}
```

Wait modes:

- `none`: return immediately after sending input;
- `idle`: return after no matching control events arrive for `quietMs`;
- `text`: return when the visible screen contains `pattern`, optionally as a
  regular expression.

Waits are capped at 20 seconds. `settled: true` only means the pane was quiet
for the requested interval; it does not claim that the process or command has
finished. Use `tmux_read` with the returned cursor to continue following output.

Attachments to the same OS user and tmux session share one native control-mode
client. Each caller gets an independent lease. Idle leases expire after 30
minutes, output is kept in a bounded event buffer, and `droppedEvents` reports
when a cursor fell behind that buffer.

`tmux_detach` only closes the lease. It never kills the tmux session.
