# Security Model

Sentinel is local-first, but can be exposed remotely when properly configured.

## Authentication

When `token` is configured, all HTTP and WS requests require authentication.

### HTTP

Use bearer token:

```http
Authorization: Bearer <token>
```

### WebSocket

WS auth is supported via subprotocol (recommended) and bearer header fallback.

- Protocol: `sentinel.v1`
- Optional auth protocol: `sentinel.auth.<base64url-token>`

Example protocol list from frontend:

```text
sentinel.v1, sentinel.auth.<base64url-token>
```

## Origin Validation

`allowed_origins` can be explicitly configured. If omitted, same-host origin checks apply.

Recommendations:

- Set explicit origins when using reverse proxies.
- Keep token required for any non-local binding.

## Remote Exposure Baseline

If `listen = "0.0.0.0:4040"`:

- Always set `token`.
- Always set `allowed_origins`.
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
