# Settings

Settings is Sentinel's local control plane. It is a first-class workspace for
changing the running deployment without turning configuration into a second
operational domain.

Open Settings from the utility action at the bottom of the desktop rail or from
the compact action in each primary mobile header. Settings does not add a sixth
destination to the primary navigation.

## Sections

The current workspace has seven deep-linkable sections:

- `/settings/experience` — terminal theme, timezone, and date format.
- `/settings/operations` — Watchtower collection, Runbook concurrency, and
  daemon log level.
- `/settings/integrations` — MCP endpoint state and scheduled health-report
  delivery.
- `/settings/accounts` — read-only OS-account inventory, target allowlist, root
  gate, and user-switch method.
- `/settings/access` — listener, shared token, browser origins, trusted proxies,
  cookie policy, and manual anti-lockout recovery.
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

<img src="assets/images/desktop-settings-operations.png" alt="Desktop Settings Operations for the fictitious Orbital Station deployment, showing the local control-plane navigation, Watchtower fields, exact source badges, validation limits, and restart-based lifecycle" />

This capture comes from the reproducible Orbital Station fixture. Its values
and deployment identity are fictional; no host configuration or session data
is present.

## Integrations and write-only secrets

Integrations has two functional groups:

- **MCP** controls the desired endpoint state, reports whether the shared token
  is ready, and provides ready-to-copy client snippets. Token changes live in
  Access so they always use the anti-lockout flow.
- **Health report** controls the delivery cron and write-only webhook URL.

The Access token and Integration webhook controls never preload an existing
value. They expose only `Configured` or `Not configured` and three explicit
actions:

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
with a token. If no token exists, Integrations links to Access and cannot stage
a second token mutation path. After a token is configured in Access and the
daemon restarts, MCP can be enabled from Integrations. The UI distinguishes
`Available`, `Disabled`, and `Pending restart`.

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

## Guarded access configuration

Access edits the complete network and authentication boundary as one candidate:

- listener host and port;
- the shared write-only `server.token`;
- allowed browser origins and trusted proxy IPs/CIDRs;
- secure-cookie policy and the explicit insecure-cookie exception.

Every Access PATCH includes an absolute `reconnectOrigin` calculated from the
current browser scheme and the candidate listener. A loopback or specific
listener uses the candidate host. A wildcard listener such as `0.0.0.0` or
`::` preserves the current browser hostname and changes only the port.

Before replacing the config, Sentinel validates the complete effective
candidate, including environment overrides. When the listener changes, it also
performs a bind preflight. A bind already held by the running Sentinel endpoint
is distinguished from a conflict on another endpoint; an external conflict is
rejected without changing the config revision, file, or live state.

The review dialog lists every changed config key, the reconnect target, the
manual restart requirement, and whether the browser must authenticate again.
Replacing or clearing the shared token never returns the value to the browser.
After a rotation is submitted, the replacement is removed from the form before
the request completes. The current process and auth cookie continue using the
startup token until restart; after restart the normal authentication gate asks
for the new token.

Remote candidates cannot omit or clear the token, cannot omit all allowed
origins, and must satisfy the cookie policy. An `always` secure cookie is
incompatible with an HTTP reconnect target. `cookie_secure = "never"` with
remote token authentication requires the explicit
`allow_insecure_cookie = true` exception.

Settings never restarts Sentinel and never rolls back automatically. Before
saving, the Access section displays the exact config path, adjacent `.bak`
path, restore command, effective validation command, and scope-specific restart
command when a managed deployment is detected. Keep those commands reachable
outside the SPA before restarting.

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

<p align="center">
  <img src="assets/images/mobile-settings-experience.png" alt="Mobile Settings Experience in the fictitious Orbital Station fixture, showing the seven-section control-plane navigation and browser-owned terminal theme controls" width="390" />
</p>

The mobile capture uses the same synthetic fixture and typed contract as the
desktop view.

## Related contracts

- [Configuration](/reference/configuration.md) defines precedence, validation,
  persistence, and field ownership.
- [HTTP API](/reference/http-api.md) defines the typed Settings transaction and
  ETag concurrency contract.
- [Security Model](/guide/security.md) defines secret handling, remote exposure,
  host accounts, and recovery boundaries.
- [Service and Autoupdate](/operations/service-and-autoupdate.md) defines manual
  restart ownership for managed and standalone deployments.
