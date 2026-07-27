# Services

![Desktop services](assets/images/desktop-services.png)

Dedicated service management page at `/services`, part of the [Ops Control Plane](/features/ops-control-plane.md). Sentinel monitors and controls host services via systemd (Linux) and launchd (macOS).

## Tracked Services

Sentinel tracks these built-in identities directly from the installed runtime:

- `sentinel` — the Sentinel server process
- `sentinel-updater` — the autoupdate timer

A built-in appears only when its canonical systemd unit or launchd label exists in
the scope observable by the Sentinel process. A non-root Linux installation
checks user and system scopes; root checks only system scope. If the same
built-in exists in both Linux scopes, Sentinel reports a deployment conflict
instead of choosing one silently.

Built-ins are never stored in `ops_custom_services` and cannot be unpinned or
deleted. Custom services can be registered via API or the UI to add them to the
tracked set.

## Service Browse

Browse discovers manageable units on the host and annotates them with tracking status. On Linux, the default view focuses on `service` units and can be expanded with the type filter to include `timer`, `socket`, `target`, and other systemd unit kinds. On macOS, Browse lists launchd jobs.

`GET /api/ops/services/browse` returns a list where each entry contains:

- `unit` — systemd unit name or launchd label
- `description` — human-readable service description
- `unitType` — discovered unit kind (`service`, `timer`, `target`, `job`, etc.)
- `activeState` — current runtime state (active, inactive, failed, etc.)
- `enabledState` — whether the unit is enabled
- `manager` — `systemd` or `launchd`
- `scope` — `user` or `system`
- `tracked` — whether this unit is in the tracked set
- `trackedName` — the registered name, if tracked
- `trackingMode` — `builtin` for runtime-owned identities or `custom` for
  explicitly registered services

From the browse view, any service can be started, stopped, restarted, inspected, or have its logs viewed without needing to track it first.

## Custom Services

Register a custom service to include it in the tracked set:

```
POST /api/ops/services
```

```json
{
  "name": "my-service",
  "displayName": "My Service",
  "manager": "systemd",
  "unit": "my-service.service",
  "scope": "user"
}
```

Defaults: `manager` defaults to `systemd`, `scope` defaults to `user`, `displayName` defaults to `name`.

Stored in the `ops_custom_services` table.

The reserved names and units/labels for `sentinel` and `sentinel-updater` cannot
be registered as custom services. The API returns
`409 OPS_SERVICE_BUILTIN` for those attempts.

Remove a tracked custom service:

```
DELETE /api/ops/services/{service}
```

Deleting a built-in identity returns `409 OPS_SERVICE_BUILTIN`.

## Unit-Level Controls

Direct actions on any unit by reference, without requiring it to be tracked:

**Action** — start, stop, or restart a unit:

```
POST /api/ops/services/unit/action
```

```json
{
  "unit": "my-service.service",
  "scope": "user",
  "manager": "systemd",
  "action": "start"
}
```

**Status inspect**:

```
GET /api/ops/services/unit/status?unit=my-service.service&scope=user&manager=systemd
```

**Logs**:

```
GET /api/ops/services/unit/logs?unit=my-service.service&scope=user&manager=systemd&lines=200
```

## Named Service Actions

For tracked services, use the name-based endpoints:

```
POST /api/ops/services/{service}/action
```

```json
{ "action": "restart" }
```

Supported actions: `start`, `stop`, `restart`.

**Status inspect**:

```
GET /api/ops/services/{service}/status
```

Returns service properties, summary, raw output, and a checked-at timestamp.

**Logs**:

```
GET /api/ops/services/{service}/logs?lines=50
```

## Realtime Events

Service state changes emit events over the `/ws/events` WebSocket:

- `ops.services.updated` — full service list refresh
- `ops.overview.updated` — updated overview with service health summary

## UX Behavior

- Service actions use optimistic updates. The UI reflects the expected state immediately and reconciles when the server responds or a realtime event arrives.
- Failed actions roll back the optimistic state and generate a toast notification.
- The browse view supports filtering by state (active, inactive, failed), scope (user, system), and free-text search.
- Custom services can be pinned or unpinned directly from the browse list.
  Built-ins retain status, logs, inspect, and service actions but do not expose
  Unpin.

## Frontend

The dedicated `/services` route provides a full-page service management experience.

- Services uses the full application width without a secondary sidebar. Pinned/tracked units remain available through the `Pinned` browse filter, with live status indicators in the service list.
- The main panel has a stats header showing total, active, and failed service counts, followed by a browse panel.
- The browse panel discovers all host services and supports filtering by state (active/inactive/failed), scope (user/system), and free-text search.
- Per-service actions include start, stop, restart, status inspect, and logs
  view. Custom services can be pinned or unpinned directly from Browse;
  runtime-owned built-ins cannot.
- Service status and logs open in modal dialogs.
- Real-time updates arrive via WebSocket events (`ops.services.updated`, `ops.overview.updated`), keeping the browse panel in sync without polling.

## API Endpoints

- `GET /api/ops/services`
- `GET /api/ops/services/browse`
- `GET /api/ops/services/discover`
- `POST /api/ops/services`
- `DELETE /api/ops/services/{service}`
- `POST /api/ops/services/{service}/action`
- `GET /api/ops/services/{service}/status`
- `GET /api/ops/services/{service}/logs`
- `POST /api/ops/services/unit/action`
- `GET /api/ops/services/unit/status`
- `GET /api/ops/services/unit/logs`
