# CLI Reference

## Root Commands

```bash
sentinel [serve]
sentinel service <install|uninstall|status|autoupdate>
sentinel doctor
sentinel recovery <list|restore>
sentinel update <check|apply|status>
sentinel --help
sentinel --version | -v | version
```

## `sentinel serve`

Start HTTP server using config/env values.

```bash
sentinel serve
```

## `sentinel service`

### Install

```bash
sentinel service install --exec PATH --enable=true --start=true
```

Flags:

- `--exec`: binary path for service unit (optional, defaults to current executable).
- `--enable`: enable at boot/login.
- `--start`: start immediately.

### Uninstall

```bash
sentinel service uninstall --disable=true --stop=true --remove-unit=true
```

Flags:

- `--disable`: disable auto-start.
- `--stop`: stop service.
- `--remove-unit`: remove managed unit/plist.

### Status

```bash
sentinel service status
```

Prints service file path, existence, manager availability, enabled and active state.

## `sentinel service autoupdate`

### Install timer/agent

```bash
sentinel service autoupdate install \
  --exec PATH \
  --enable=true \
  --start=true \
  --service sentinel \
  --scope auto|user|system|launchd \
  --on-calendar daily \
  --randomized-delay 1h
```

### Uninstall

```bash
sentinel service autoupdate uninstall \
  --disable=true --stop=true --remove-unit=true \
  --scope auto|user|system|launchd
```

### Status

```bash
sentinel service autoupdate status --scope auto|user|system|launchd
```

## `sentinel doctor`

```bash
sentinel doctor
```

Outputs host/runtime diagnosis: OS/arch, listen addr, data dir, tmux path, service manager status, and managed unit states.

## `sentinel recovery`

### List

```bash
sentinel recovery list --state killed,restored --limit 100
```

Allowed states: `running`, `killed`, `restoring`, `restored`, `archived`.

### Restore

```bash
sentinel recovery restore \
  --snapshot 42 \
  --mode confirm \
  --conflict rename \
  --target my-session \
  --wait=true
```

- `--mode`: `safe|confirm|full`
- `--conflict`: `rename|replace|skip`

## `sentinel update`

### Check

```bash
sentinel update check --repo owner/name --api URL --os linux --arch amd64
```

### Apply

```bash
sentinel update apply \
  --repo owner/name \
  --api URL \
  --exec PATH \
  --os linux \
  --arch amd64 \
  --allow-downgrade=false \
  --allow-unverified=false \
  --restart=false \
  --service sentinel \
  --scope auto|user|system|launchd|none
```

### Status

```bash
sentinel update status
```

## Exit Codes

- `0`: success
- `1`: runtime/operation error
- `2`: invalid CLI usage or flags
