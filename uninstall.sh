#!/usr/bin/env bash
set -euo pipefail

# Sentinel uninstaller
# Usage: curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/uninstall.sh | bash
#
# Environment variables:
#   INSTALL_DIR          - Binary install directory (default: ~/.local/bin, or /usr/local/bin when root)

IS_ROOT=0
if [ "$(id -u)" -eq 0 ]; then
    IS_ROOT=1
fi

if [ -z "${INSTALL_DIR:-}" ]; then
    if [ "$IS_ROOT" -eq 1 ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi

SENTINEL="${INSTALL_DIR}/sentinel"

# --- Colors ---
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf "${CYAN}%s${RESET}\n" "$*"; }
ok()    { printf "${GREEN}%s${RESET}\n" "$*"; }
warn()  { printf "${YELLOW}warning: %s${RESET}\n" "$*" >&2; }
err()   { printf "${RED}error: %s${RESET}\n" "$*" >&2; exit 1; }

# --- Detect platform ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux|darwin) ;;
    *) err "unsupported OS: $OS" ;;
esac

# --- Uninstall services ---
if [ -x "$SENTINEL" ]; then
    info "Removing autoupdate service..."
    if "$SENTINEL" service autoupdate uninstall --disable=true --stop=true --remove-unit=true 2>/dev/null; then
        ok "Autoupdate service removed"
    else
        warn "autoupdate service was not installed (skipped)"
    fi

    info "Removing sentinel service..."
    if "$SENTINEL" service uninstall --disable=true --stop=true --remove-unit=true; then
        ok "Sentinel service removed"
    else
        warn "failed to uninstall service; continuing with binary removal"
    fi
elif command -v systemctl >/dev/null 2>&1; then
    # Binary missing but systemd units may still exist — clean up manually.
    warn "sentinel binary not found at ${SENTINEL}; attempting manual service cleanup"
    if [ "$IS_ROOT" -eq 1 ]; then
        systemctl disable --now sentinel 2>/dev/null || true
        systemctl disable --now sentinel-updater.timer 2>/dev/null || true
        rm -f /etc/systemd/system/sentinel.service
        rm -f /etc/systemd/system/sentinel-updater.service
        rm -f /etc/systemd/system/sentinel-updater.timer
        rm -f /etc/needrestart/conf.d/sentinel.conf
        systemctl daemon-reload 2>/dev/null || true
    else
        systemctl --user disable --now sentinel 2>/dev/null || true
        systemctl --user disable --now sentinel-updater.timer 2>/dev/null || true
        rm -f "$HOME/.config/systemd/user/sentinel.service"
        rm -f "$HOME/.config/systemd/user/sentinel-updater.service"
        rm -f "$HOME/.config/systemd/user/sentinel-updater.timer"
        systemctl --user daemon-reload 2>/dev/null || true
    fi
    ok "Service units cleaned up"
fi

# --- Remove binary ---
if [ -f "$SENTINEL" ]; then
    rm -f "$SENTINEL"
    ok "Removed ${SENTINEL}"
else
    warn "binary not found at ${SENTINEL} (already removed?)"
fi

echo ""
ok "Sentinel uninstalled successfully"
