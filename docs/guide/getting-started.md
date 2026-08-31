# Getting Started

This path ends at Sentinel's first useful outcome: a current read in **Now**
and a precise handoff to the module that owns the next question.

## Requirements

- Linux or macOS.
- `tmux` for terminal workspace features.
- A browser with WebSocket support.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

The installer selects user or system scope from the invoking account and
installs the matching systemd or launchd service. If you install through Go
instead, the binary is available but service installation remains a separate
operation:

```bash
go install github.com/opus-domini/sentinel/cmd/sentinel@latest
```

## Verify a Downloaded Release

Every release ships a `sentinel-<version>-checksums.txt` covering each archive
and SBOM by SHA-256, plus a keyless cosign bundle over that checksums file.
`install.sh` already verifies the checksum; verify the signature when you
download archives by hand.

```bash
VERSION=0.11.1
cosign verify-blob \
  --bundle "sentinel-${VERSION}-checksums.txt.bundle" \
  --certificate-identity-regexp '^https://github\.com/opus-domini/actions/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "sentinel-${VERSION}-checksums.txt"
```

Then check your archive against the verified checksums file:

```bash
sha256sum --ignore-missing -c "sentinel-${VERSION}-checksums.txt"
```

The signature is produced inside the shared release workflow, so Fulcio issues
the certificate against that workflow's `job_workflow_ref`. The signing
identity is therefore `opus-domini/actions`, not `opus-domini/sentinel`.

## Verify the Runtime

Run the diagnostics before debugging in the browser:

```bash
sentinel doctor
sentinel service status
```

`doctor` checks the effective deployment and configuration. `service status`
confirms the installed daemon lifecycle. Resolve failures here before treating
an empty or unreachable UI as a product-state problem.

## Open the Workspace

Open `http://127.0.0.1:4040`.

When `server.token` is configured, Sentinel shows a dedicated authentication
gate. Enter the shared secret there. The gate stores the browser credential for
same-origin requests; there is no Sentinel username or account selection.

If the workspace does not load, use [Common Issues](/troubleshooting/common-issues.md)
before changing config.

## Read Now First

The home route is **Now**. Read it in this order:

1. **Host posture** — whether current Services or Metrics evidence is healthy,
   at risk, or unable to confirm state.
2. **Confidence** — whether Tmux, Services, Metrics, and Runbooks all supplied
   current evidence.
3. **Needs attention** — the bounded decisions that require an operator.
4. **In progress** — active procedures and Tmux sessions carrying live context.

Then follow the owner link:

- failed service → [Services](/features/services.md) for condition and logs;
- pressure signal → [Metrics](/features/metrics.md) for live evidence;
- approval or execution → [Runbooks](/features/runbooks.md);
- active shell context → [Tmux Workspace](/features/tmux-workspace.md).

That handoff is the first value of Sentinel. You do not need to configure every
module before using the workspace.

## Add Only What the Host Needs

Create a tracked service when a unit deserves repeated condition, log, and
lifecycle access. Create a Runbook when a recurring procedure needs explicit
steps, parameters, approval, or an auditable receipt. Use OS account targeting
only when Tmux work must run as another allowed host account.

The owner guides describe those workflows:

- [Services](/features/services.md)
- [Runbooks](/features/runbooks.md)
- [OS Account Targeting](/features/os-account-targeting.md)

## Configure Remote Access Deliberately

Initialize and edit the canonical config:

```bash
sentinel config init
sentinel config edit
sentinel config validate
```

Loopback is the safe default:

```toml
[server]
host = "127.0.0.1"
port = 4040
token = ""
allowed_origins = []
trusted_proxies = []
```

If Sentinel listens beyond loopback, configure both controls:

```toml
[server]
host = "0.0.0.0"
port = 4040
token = "replace-with-a-strong-shared-secret"
allowed_origins = ["https://sentinel.example.com"]
trusted_proxies = []
```

Sentinel does not terminate TLS. Put HTTPS and any broader network access
policy at the trusted edge. See [Security Model](/guide/security.md).

## Optional Daily Autoupdate

Autoupdate operates the Sentinel installation; it is not part of the daily
host-evidence loop.

```bash
sentinel service autoupdate install
sentinel service autoupdate status
```

For a system deployment:

```bash
sentinel service autoupdate install --scope system
```
