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

## Origin Validation

`allowed_origins` can be explicitly configured. If omitted, same-host origin checks apply.

Recommendations:

- Set explicit origins when using reverse proxies.
- Keep token required for any non-local binding.

## Remote Exposure Baseline

If `listen = "0.0.0.0:4040"`:

- Always set `token`.
- Always set `allowed_origins`.
- Sentinel refuses startup when `token` is missing on non-loopback binds.
- Missing `allowed_origins` now emits a startup warning (recommended to configure).
- Prefer private network overlay (VPN/Tailscale) or authenticated tunnel.
- Avoid direct public exposure without additional network controls.

## Transport Notes

- Sentinel itself serves HTTP; TLS termination is typically handled by a reverse proxy.
- Protect upstream with HTTPS and strict origin policy.

## Security-Related Error Codes

Common API auth/origin responses:

- `401 UNAUTHORIZED`
- `403 ORIGIN_DENIED`

Guardrail enforcement can also block dangerous actions:

- `409 GUARDRAIL_BLOCKED`
- `428 GUARDRAIL_CONFIRM_REQUIRED`
