# Settings

Settings is Sentinel's local control plane. It is a first-class workspace for
changing the running deployment without turning configuration into a second
operational domain.

Open Settings from the utility action at the bottom of the desktop rail or from
the compact action in each primary mobile header. Settings does not add a sixth
destination to the primary navigation.

## Sections

The current workspace has six deep-linkable sections:

- `/settings/experience` — terminal theme, timezone, and date format.
- `/settings/operations` — Watchtower collection, Runbook concurrency, and
  daemon log level.
- `/settings/integrations` — MCP access, shared token lifecycle, and scheduled
  health-report delivery.
- `/settings/accounts` — read-only OS-account inventory, target allowlist, root
  gate, and user-switch method.
- `/settings/storage` — a read-only storage overview and a link to maintenance.
- `/settings/diagnostics` — runtime, deployment, config path, restart state, and
  PWA install/update controls.

Opening `/settings` redirects to Experience. Unknown section names also return
to Experience instead of exposing an incomplete configuration surface.

## Experience and ownership

Each server-managed field shows where its effective value comes from and when
the change applies:

- `Default`, `Config file`, or `Environment` describes the source.
- `Live`, `Partially live`, `After restart`, or `Restart pending` describes the
  lifecycle.

Environment-owned values are read-only. Theme is explicitly marked
`This browser`; PWA state belongs to `This device`.

Timezone accepts `Local` or a valid IANA name such as
`America/Sao_Paulo`. Locale remains a closed selection supplied by the typed
settings API.

## Operational configuration

Operations groups restart-based controls into one draft:

- Watchtower enabled state, collection interval, pane-tail line count, capture
  timeout, and journal retention;
- maximum concurrent Runbook executions;
- daemon log level from the server-provided `debug`, `info`, `warn`, and
  `error` options.

Duration fields use compact values such as `150ms`, `1s`, and `1m`. Integer and
duration limits come from the typed Settings response and are enforced again by
the backend before any file write.

The sticky save bar lists the exact config keys and old/new values. `Discard`
returns every field to the current server snapshot without a PATCH. Navigating
within Settings, leaving the workspace, using browser Back, or closing the page
with an unsaved draft is guarded.

After a successful save, Sentinel reports the changed keys and deployment
scope. The restart command is displayed and can be copied when the config path
matches an installed user or system service. Sentinel does not restart itself;
the running process keeps its startup values until the operator performs the
manual restart.

## Integrations and write-only secrets

Integrations has two functional groups:

- **MCP** controls the desired endpoint state, the shared `server.token`, and
  ready-to-copy client snippets.
- **Health report** controls the delivery cron and write-only webhook URL.

The token and webhook controls never preload an existing value. They expose
only `Configured` or `Not configured` and three explicit actions:

- `Keep` leaves the saved value unchanged and sends no secret.
- `Replace` accepts a new value once.
- `Clear` removes the saved value and sends no secret.

Replacement values are removed from the form as soon as Save is submitted,
including after validation errors or conflicts. They are not written to the
Settings response, query cache, toast, clipboard snippet, or browser storage.
Environment-owned secrets are configured but read-only.

Cron expressions are parsed only by Sentinel. After a valid save, the response
includes the next activation calculated by the backend; the browser does not
implement a second cron parser. Both the health-report schedule and webhook
apply after restart.

MCP disable remains live. MCP enable is live when the running process started
with a token. When a token is newly replaced, Sentinel persists both the token
and desired MCP state but keeps the endpoint unavailable until manual restart.
The UI distinguishes `Available`, `Disabled`, and `Pending restart`.

## OS-account targeting

Accounts configures the existing `[multi_user]` process-targeting boundary
without becoming an operating-system account manager. The process identity and
eligible account inventory are loaded by the daemon at startup and displayed
read-only. Settings never creates, deletes, renames, or changes an OS account.

`allowed_users` is a closed selection from that inventory. An empty list means
all detected accounts are eligible; it does not mean no accounts. Unknown and
duplicate names are rejected by the browser and again by the typed PATCH
endpoint. Environment-owned fields remain visible but read-only.

Root is blocked independently by default. Enabling `allow_root_target` requires
an explicit risk confirmation. For an explicit file-owned allowlist, enabling
root includes it and disabling root removes it so the persisted policy cannot
contradict the gate. If the allowlist belongs to the environment, Settings
cannot rewrite it and the effective environment allowlist still applies.

The switch method is a closed `sudo` or `systemd-run` choice. Settings reports
whether the required executables were found in the daemon PATH, but that
capability is advisory: Sentinel cannot inspect or grant sudo policy. All three
account controls apply after a manual restart, and saving them never starts,
stops, or changes a tmux session.

## Storage maintenance

Settings only summarizes storage. Destructive cleanup remains isolated at
`/maintenance/storage`, where Sentinel shows the exact eligible and protected
row counts and asks for confirmation.

Active runbook jobs are never eligible for cleanup. See
[Storage and flush](../operations/storage-and-flush.md) for transaction and
protection guarantees.

## Responsive behavior

Desktop uses an internal side navigation. Compact viewports stack navigation
above the active section and keep one vertical scroll owner. The workspace uses
the existing visual viewport tracking, safe-area handling, and keyboard-aware
mobile shell; all Settings actions have at least a 44 px target.
