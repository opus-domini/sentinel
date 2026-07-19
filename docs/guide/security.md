# Security Model

Sentinel is local-first, but can be exposed remotely when properly configured.

![Desktop settings token](assets/images/desktop-settings-token.png)

## Authentication

When `token` is configured, all HTTP and WS requests require authentication.

### HTTP

Authentication uses an HttpOnly cookie set via the token endpoint:

```http
PUT /api/auth/token
Content-Type: application/json

{ "token": "<token>" }
```

On success, the server sets the `sentinel_auth` HttpOnly cookie. All subsequent HTTP requests are authenticated via this cookie.

### WebSocket

WS connections authenticate via the same `sentinel_auth` HttpOnly cookie. The browser includes the cookie automatically on connection.

- Protocol: `sentinel.v1`

### MCP

`/mcp` requires `Authorization: Bearer <server.token>` on every request. The
browser authentication cookie is intentionally not accepted for MCP clients.
The endpoint is absent (`404`) while `[mcp].enabled` is false, and Sentinel
refuses to enable it when `server.token` is empty.

MCP requests pass through the same origin policy and multi-user target checks
as the rest of Sentinel. Exposing MCP gives an authenticated client the ability
to create tmux sessions and send input to panes, so keep it on a private network
overlay or behind an authenticated TLS endpoint.

## Origin Validation

`allowed_origins` can be explicitly configured. If omitted, same-host origin checks apply on loopback binds.
For non-loopback binds, Sentinel requires at least one explicit allowed origin at startup.

Recommendations:

- Set explicit origins when using reverse proxies.
- Keep token required for any non-local binding.

## Trusted Proxies

Sentinel trusts `X-Forwarded-Proto` from IPv4 and IPv6 loopback proxies by
default. Add IPs or CIDRs to `trusted_proxies` only when the direct proxy peer
is not local.

```toml
[server]
trusted_proxies = ["10.0.0.0/8"]
```

Requests from untrusted remotes cannot force HTTPS origin/cookie decisions with forwarded headers.

## Remote Exposure Baseline

If `server.host = "0.0.0.0"`:

- Always set `token`.
- Always set `allowed_origins`.
- Sentinel refuses startup when `token` is missing on non-loopback binds.
- Sentinel refuses startup when `allowed_origins` is missing on non-loopback binds.
- Prefer private network overlay (VPN/Tailscale) or authenticated tunnel.
- Avoid direct public exposure without additional network controls.

## Transport Notes

- Sentinel itself serves HTTP; TLS termination is typically handled by a reverse proxy.
- Protect upstream with HTTPS and strict origin policy.

## Multi-User Session Security

When `[multi_user]` is enabled, session creation can target other OS users. Validation follows a two-tier model:

1. **Allowlist**: if `allowed_users` is configured, only listed users are permitted.
2. **System users fallback**: if no allowlist is set, users are validated against `/etc/passwd` entries with UID >= 1000.

Additional controls:

- `allow_root_target` gate (defaults to `false`) — must be explicitly enabled to allow targeting the root user.
- `ErrNoSystemUsers` blocks all user switching when system users cannot be loaded from `/etc/passwd`.
- Validation failure returns `403 USER_NOT_ALLOWED`.
- All multi-user session creations are logged.

## Security-Related Error Codes

Common API auth/origin responses:

- `401 UNAUTHORIZED`
- `403 ORIGIN_DENIED`
- `403 USER_NOT_ALLOWED`

Authorization failures are returned before protected HTTP and WebSocket handlers run.
