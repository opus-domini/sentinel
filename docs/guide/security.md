# Security Model

Sentinel is local-first and assumes one trusted operator boundary around one
host. Remote access is supported only when the shared credential, origin policy,
transport, and network exposure are configured together.

## Trust Boundary

- `server.token` is one shared operator secret. It authenticates access; it
  does not identify an individual.
- Sentinel has no application users, per-user sessions, roles, RBAC, tenants,
  resource scopes, or attributable approval principals.
- [OS Account Targeting](/features/os-account-targeting.md) chooses the host
  account used by a Tmux process. An OS account is not a Sentinel identity.
- MCP uses the same shared secret and trust boundary. It does not create an
  agent identity or grant per-agent scopes.

Treat every client that knows the token as a trusted operator with access to
the enabled Sentinel surfaces.

Configuration secrets are never safe diagnostic data. CLI output redacts the
shared token and webhook URLs, and delivery logs report only whether a webhook
is configured and whether delivery succeeded. Sentinel does not log the full
webhook URL because its path or query may contain a credential.

The Settings HTTP contract follows the same rule. `GET /api/ops/settings`
returns only non-secret values and `configured` booleans; it never returns raw
TOML, `server.token`, or a webhook URL. PATCH can mutate secrets only through
explicit `keep`, `replace`, and `clear` actions. `replace` is submitted once and
removed from the form immediately; `keep` and `clear` carry no value. No secret
is copied into the Settings response, query cache, toast, connection snippet,
or browser storage.

Every PATCH requires the ETag from the latest GET so a stale browser tab cannot
silently overwrite a concurrent CLI or browser change. A conflict refetches
safe metadata and discards any submitted replacement value instead of trying
to replay it.

Environment-owned secrets are reported as configured and read-only. A forged
PATCH is rejected by the canonical config service before any file write.

## Request Surfaces

When `server.token` is configured, the request boundaries are:

| Surface | Origin policy | Credential |
| --- | --- | --- |
| SPA shell and `/manifest.webmanifest` | Required | None; these remain loadable so the browser can present the auth gate |
| `PUT`/`DELETE /api/auth/token` | Required | Submitted token for login; no existing cookie required |
| Other `/api/*` routes | Required | `sentinel_auth` cookie |
| `/ws/tmux`, `/ws/events`, `/ws/logs` | Required | `sentinel_auth` cookie |
| `/mcp` | Required | `Authorization: Bearer <server.token>` |

When no token is configured, API and WebSocket credential checks are
effectively open inside the configured origin/network boundary. MCP cannot be
enabled without a token.

## Browser Authentication

The browser presents a dedicated authentication gate before the application.
Enter the shared token there; Settings is not available until authentication
succeeds.

```http
PUT /api/auth/token
Content-Type: application/json

{ "token": "<shared-token>" }
```

On success, Sentinel sets the `sentinel_auth` HttpOnly cookie. The browser sends
that cookie automatically to protected HTTP and WebSocket routes. Clearing the
credential uses `DELETE /api/auth/token`.

WebSocket connections use subprotocol `sentinel.v1`; tokens never belong in WS
query parameters.

## MCP Authentication

Every `/mcp` request must send:

```http
Authorization: Bearer <server.token>
```

The browser cookie is intentionally not accepted for MCP. The endpoint returns
`404` while `[mcp].enabled` is false, and configuration validation refuses MCP
when `server.token` is empty.

There is no separate MCP token, agent principal, role, or scope. MCP also has no
tool for approving or rejecting a Runbook approval step; that decision remains
in the authenticated Sentinel UI. See [MCP Control](/features/mcp.md).

Replacing the shared token does not change the credential held by the running
process. The new value and MCP enablement remain restart pending until Sentinel
restarts. Clearing the token requires a candidate configuration in which MCP is
disabled and all remote-exposure rules remain valid.

Health-report webhook failures expose only a delivery category or HTTP status.
The URL, path, query, userinfo, and fragments are never included in returned
errors or logs.

## Origin Validation

`allowed_origins` can be configured explicitly. If omitted on a loopback bind,
Sentinel checks for the same host and scheme. A non-loopback bind requires at
least one explicit allowed origin at startup.

Recommendations:

- Set explicit origins when using a reverse proxy.
- Keep a token required for every non-local binding.
- Do not confuse an allowed origin with authentication; both checks matter.

## Trusted Proxies

Sentinel trusts `X-Forwarded-Proto` only from IPv4 or IPv6 loopback proxies by
default. Add IPs or CIDRs to `trusted_proxies` when the direct proxy peer is not
local:

```toml
[server]
trusted_proxies = ["10.0.0.0/8"]
```

Requests from untrusted remotes cannot force HTTPS origin or cookie decisions
with forwarded headers.

## OS Account Targeting

The `[multi_user]` config section controls process targeting, despite its
historical internal name:

1. When `allowed_users` is non-empty, a target must be in that allowlist.
2. Otherwise, a target must be in the eligible OS-account inventory loaded at
   startup.
3. `root` remains blocked unless `allow_root_target = true`.
4. An unavailable OS-account inventory blocks non-default account targeting
   with `ErrNoSystemUsers`.

These checks do not create a login, role, tenant, or per-user audit identity in
Sentinel.

## Remote Exposure Baseline

For `server.host = "0.0.0.0"`:

- Set `token`.
- Set `allowed_origins`.
- Prefer a private overlay network, VPN, or authenticated tunnel.
- Terminate TLS at a trusted reverse proxy when HTTPS is needed.
- Avoid direct public exposure without additional network controls.

Sentinel refuses a non-loopback startup when either the token or allowed origins
are missing.

The same remote-exposure and cookie rules run during daemon startup, CLI
validation, and managed configuration saves. A configuration that would expose
Sentinel without the required token and origins is rejected before it is
persisted. Environment-owned security fields must be changed in the process
environment; file-based updates cannot shadow them.

## Security-Related Error Codes

- `401 UNAUTHORIZED` — missing or invalid shared credential
- `403 ORIGIN_DENIED` — browser origin is not allowed
- `403 UNTRUSTED_PROXY` — forwarded HTTPS came from an untrusted direct peer
- `403 USER_NOT_ALLOWED` — requested OS account failed target validation

Authorization and origin failures are returned before protected handlers run.
