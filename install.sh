#!/usr/bin/env bash
set -euo pipefail

# Sentinel installer
# Usage: curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
#
# Environment variables:
#   INSTALL_DIR   - Binary install directory (default: ~/.local/bin)
#   VERSION       - Specific version to install (default: latest)

REPO="opus-domini/sentinel"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
SERVICE_DIR="$HOME/.config/systemd/user"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf "${CYAN}%s${RESET}\n" "$*"; }
ok()    { printf "${GREEN}%s${RESET}\n" "$*"; }
err()   { printf "${RED}error: %s${RESET}\n" "$*" >&2; exit 1; }

# --- Detect platform ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="arm"   ;;
    *)       err "unsupported architecture: $ARCH" ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) err "unsupported OS: $OS" ;;
esac

# --- Check dependencies ---
command -v curl  >/dev/null 2>&1 || err "curl is required but not installed"
command -v tar   >/dev/null 2>&1 || err "tar is required but not installed"
command -v tmux  >/dev/null 2>&1 || err "tmux is required but not installed"

# --- Get version ---
if [ -z "${VERSION:-}" ]; then
    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | cut -d'"' -f4)
    [ -n "$VERSION" ] || err "could not determine latest version"
fi

info "Installing Sentinel ${VERSION} (${OS}/${ARCH})..."

# --- Download and extract ---
TARBALL="sentinel-${VERSION#v}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading ${URL}..."
curl -fsSL "$URL" -o "${TMP}/${TARBALL}" || err "download failed — check that version ${VERSION} exists"
tar -xzf "${TMP}/${TARBALL}" -C "$TMP" || err "extraction failed"

# --- Install binary ---
mkdir -p "$INSTALL_DIR"
install -m755 "${TMP}/sentinel" "${INSTALL_DIR}/sentinel"
ok "Installed sentinel to ${INSTALL_DIR}/sentinel"

# --- Install systemd user service (Linux only) ---
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
    mkdir -p "$SERVICE_DIR"

    # Adjust ExecStart if using a non-default install dir
    EXEC_START="${INSTALL_DIR}/sentinel"
    cat > "${SERVICE_DIR}/sentinel.service" << EOF
[Unit]
Description=Sentinel - tmux session manager
Documentation=https://github.com/${REPO}
StartLimitIntervalSec=60
StartLimitBurst=4

[Service]
Type=simple
ExecStart=${EXEC_START}
Restart=on-failure
RestartSec=2
# Preserve tmux server/sessions across sentinel restarts.
# Default systemd KillMode=control-group would terminate tmux too.
KillMode=process
Environment=SENTINEL_LOG_LEVEL=info
Environment=TERM=xterm-256color
Environment=LANG=C.UTF-8
SystemCallArchitectures=native
NoNewPrivileges=true

[Install]
WantedBy=default.target
EOF

    systemctl --user daemon-reload

    echo ""
    ok "systemd user service installed."
    printf "\n${BOLD}  Start now:${RESET}         systemctl --user start sentinel\n"
    printf "${BOLD}  Enable on login:${RESET}   systemctl --user enable sentinel\n"
    printf "${BOLD}  View logs:${RESET}         journalctl --user -u sentinel -f\n"
    printf "\n  ${CYAN}Optional (start at boot without login):${RESET}\n"
    printf "    sudo loginctl enable-linger \$USER\n"
fi

# --- macOS hint ---
if [ "$OS" = "darwin" ]; then
    echo ""
    info "On macOS, you can create a launchd plist to start Sentinel on login."
    info "See: https://github.com/${REPO}#install-as-a-service"
fi

# --- Verify PATH ---
echo ""
if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "sentinel installed successfully"
else
    printf "${BOLD}NOTE:${RESET} %s is not in your PATH.\n" "$INSTALL_DIR"
    printf "Add it:  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
fi
