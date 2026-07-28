# Settings

Settings is Sentinel's local control plane. It is a first-class workspace for
changing the running deployment without turning configuration into a second
operational domain.

Open Settings from the utility action at the bottom of the desktop rail or from
the compact action in each primary mobile header. Settings does not add a sixth
destination to the primary navigation.

## Sections

The current workspace has four deep-linkable sections:

- `/settings/experience` — terminal theme, timezone, and date format.
- `/settings/integrations` — MCP availability and connection instructions.
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
