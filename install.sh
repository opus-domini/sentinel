#!/usr/bin/env bash
set -euo pipefail

# Sentinel installer
# Usage: curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
#
# Environment variables:
#   INSTALL_DIR          - Binary install directory (default: ~/.local/bin, or /usr/local/bin when root)
#   VERSION              - Specific version to install (default: latest)
#   SYSTEMD_TARGET_USER  - systemd template user instance when running as root (default: root)

REPO="opus-domini/sentinel"
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

USER_SERVICE_DIR="$HOME/.config/systemd/user"
SYSTEM_SERVICE_DIR="/etc/systemd/system"

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
highlight_warn() {
    printf "\n${YELLOW}${BOLD}==================== IMPORTANT ====================${RESET}\n" >&2
    printf "${YELLOW}${BOLD}%s${RESET}\n" "$*" >&2
    printf "${YELLOW}${BOLD}===================================================${RESET}\n\n" >&2
}

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
if ! command -v tmux >/dev/null 2>&1; then
    highlight_warn "tmux was not found on this host. Sentinel installed successfully, but tmux features will stay disabled until tmux is installed."
fi

# --- Get version ---
if [ -z "${VERSION:-}" ]; then
    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | cut -d'"' -f4)
    [ -n "$VERSION" ] || err "could not determine latest version"
fi
if [ "${VERSION#v}" = "${VERSION}" ]; then
    VERSION="v${VERSION}"
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

# --- Install systemd service (Linux only) ---
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
    EXEC_START="${INSTALL_DIR}/sentinel"
    ESCAPED_EXEC_START=$(printf '%s\n' "${EXEC_START}" | sed 's/[\/&]/\\&/g')

    if [ "$IS_ROOT" -eq 1 ]; then
        TARGET_USER="${SYSTEMD_TARGET_USER:-root}"
        TARGET_UNIT="sentinel@${TARGET_USER}"
        SERVICE_TEMPLATE_URL="https://raw.githubusercontent.com/${REPO}/${VERSION}/contrib/sentinel@.service"
        TEMPLATE_PATH="${TMP}/sentinel@.service"
        SERVICE_PATH="${SYSTEM_SERVICE_DIR}/sentinel@.service"

        info "Running as root: installing system-level unit template from ${SERVICE_TEMPLATE_URL}..."
        if curl -fsSL "${SERVICE_TEMPLATE_URL}" -o "${TEMPLATE_PATH}"; then
            sed "s|^ExecStart=.*$|ExecStart=${ESCAPED_EXEC_START}|" "${TEMPLATE_PATH}" > "${SERVICE_PATH}"

            if systemctl daemon-reload; then
                if systemctl start "${TARGET_UNIT}"; then
                    echo ""
                    ok "systemd system service installed and started."
                    printf "\n${BOLD}  Service unit:${RESET}      %s\n" "${TARGET_UNIT}"
                    printf "${BOLD}  Status:${RESET}            systemctl status %s\n" "${TARGET_UNIT}"
                    printf "${BOLD}  Logs:${RESET}              journalctl -u %s -f\n" "${TARGET_UNIT}"
                    printf "${BOLD}  Enable on boot:${RESET}    systemctl enable %s\n" "${TARGET_UNIT}"
                else
                    warn "installed ${SERVICE_PATH}, but failed to start ${TARGET_UNIT}"
                    warn "you can try: systemctl start ${TARGET_UNIT}"
                fi
            else
                warn "failed to run 'systemctl daemon-reload'"
                warn "service file was written to ${SERVICE_PATH}"
            fi
        else
            warn "failed to download contrib/sentinel@.service for ${VERSION}; skipping service installation"
        fi
    else
        mkdir -p "$USER_SERVICE_DIR"
        SERVICE_TEMPLATE_URL="https://raw.githubusercontent.com/${REPO}/${VERSION}/contrib/sentinel.service"
        TEMPLATE_PATH="${TMP}/sentinel.service"
        SERVICE_PATH="${USER_SERVICE_DIR}/sentinel.service"

        info "Installing systemd user service from ${SERVICE_TEMPLATE_URL}..."
        if curl -fsSL "${SERVICE_TEMPLATE_URL}" -o "${TEMPLATE_PATH}"; then
            sed "s|^ExecStart=.*$|ExecStart=${ESCAPED_EXEC_START}|" "${TEMPLATE_PATH}" > "${SERVICE_PATH}"

            if systemctl --user daemon-reload; then
                if systemctl --user start sentinel; then
                    echo ""
                    ok "systemd user service installed and started."
                else
                    warn "service installed, but failed to start user unit"
                    warn "you can try: systemctl --user start sentinel"
                fi
                printf "\n${BOLD}  Enable on login:${RESET}   systemctl --user enable sentinel\n"
                printf "${BOLD}  View logs:${RESET}         journalctl --user -u sentinel -f\n"
                printf "\n  ${CYAN}Optional (start at boot without login):${RESET}\n"
                printf "    sudo loginctl enable-linger \$USER\n"
            else
                warn "failed to run 'systemctl --user daemon-reload' (likely no active user bus)"
                warn "service file was written to ${SERVICE_PATH}"
            fi
        else
            warn "failed to download contrib/sentinel.service for ${VERSION}; skipping service installation"
        fi
    fi
fi

# --- macOS hint ---
if [ "$OS" = "darwin" ]; then
    echo ""
    info "On macOS, you can create a launchd plist to start Sentinel on login."
    info "See: https://github.com/${REPO}#after-installation-user-journey"
fi

# --- Verify PATH ---
echo ""
if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "sentinel installed successfully"
else
    printf "${BOLD}NOTE:${RESET} %s is not in your PATH.\n" "$INSTALL_DIR"
    printf "Add it:  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
fi
