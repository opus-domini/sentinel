# Sentinel Documentation Showcase

The eight showcase PNGs are reproducible captures of the real Sentinel
frontend connected to a disposable local daemon. They tell one fictitious
**Orbital Station** workflow from current risk, through owner diagnosis and an
immutable execution receipt, to live Tmux work and a healthy return to Now.
Sentinel has no satellite-specific behavior.

## Canonical Assets

- `desktop-now-risk.png`
- `desktop-services-diagnosis.png`
- `desktop-metrics-pressure.png`
- `desktop-runbooks-receipt.png`
- `desktop-tmux-mission-control.png`
- `desktop-now-healthy.png`
- `mobile-now.png`
- `mobile-tmux.png`

`showcase-manifest.tsv` records the route state, viewport, theme, scenario,
source commit, and capture time for each image. `logo.png` is independent of
the showcase.

## Recapture

From the repository root:

```bash
make docs-showcase
make docs-showcase-check
```

The capture command builds the real frontend, starts an ephemeral loopback
daemon, creates an isolated tmux server under a temporary `TMUX_TMPDIR`, and
writes validated staging output to `.artifacts/docs-showcase/`. It unsets
inherited `TMUX` and `TMUX_PANE`, verifies the isolated socket path before
arming cleanup, and never contacts the user's default tmux server or installed
Sentinel service.

Inspect all eight staged images at native resolution before replacing the
canonical files in this directory. Publish the complete series together so
the manifest and temporal story remain coherent.

## Privacy Contract

The fixture may use only the vocabulary defined by the Orbital Station
scenario. The automated check rejects host paths, the runtime user and
hostname, credentials, local network identifiers, non-reserved IP addresses,
and PID-shaped process data. A passing OCR check supplements but does not
replace native visual inspection.
