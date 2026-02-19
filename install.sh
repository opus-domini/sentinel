#!/usr/bin/env bash
set -euo pipefail

# Sentinel installer
# Usage: curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
#
# Environment variables:
#   INSTALL_DIR          - Binary install directory (default: ~/.local/bin, or /usr/local/bin when root)
#   VERSION              - Specific version to install (default: latest)
#   ENABLE_AUTOUPDATE    - Set to 1/true to install and enable daily autoupdate timer

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

is_true() {
    case "${1:-}" in
        1|true|TRUE|True|yes|YES|on|ON) return 0 ;;
        *) return 1 ;;
    esac
}

AUTOUPDATE_ENABLED=0
if is_true "${ENABLE_AUTOUPDATE:-false}"; then
    AUTOUPDATE_ENABLED=1
fi

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

SHA256_TOOL=""
if command -v sha256sum >/dev/null 2>&1; then
    SHA256_TOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA256_TOOL="shasum"
else
    warn "no checksum tool found (sha256sum/shasum); release integrity verification will be skipped"
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
CHECKSUMS_FILE="sentinel-${VERSION#v}-checksums.txt"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/${CHECKSUMS_FILE}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading ${URL}..."
curl -fsSL "$URL" -o "${TMP}/${TARBALL}" || err "download failed — check that version ${VERSION} exists"

if curl -fsSL "${CHECKSUMS_URL}" -o "${TMP}/${CHECKSUMS_FILE}"; then
    if [ -n "${SHA256_TOOL}" ]; then
        TARGET_CHECKSUM_FILE="${TMP}/sentinel-target-checksum.txt"
        awk -v target="${TARBALL}" '
            NF >= 2 {
                file = $NF
                gsub(/^\*/, "", file)
                if (file == target) {
                    print $1 "  " target
                }
            }
        ' "${TMP}/${CHECKSUMS_FILE}" > "${TARGET_CHECKSUM_FILE}"

        if [ ! -s "${TARGET_CHECKSUM_FILE}" ]; then
            err "checksum entry for ${TARBALL} was not found in ${CHECKSUMS_FILE}"
        fi

        info "Verifying release checksum..."
        if [ "${SHA256_TOOL}" = "sha256sum" ]; then
            (cd "${TMP}" && sha256sum -c "$(basename "${TARGET_CHECKSUM_FILE}")") || err "checksum verification failed"
        else
            (cd "${TMP}" && shasum -a 256 -c "$(basename "${TARGET_CHECKSUM_FILE}")") || err "checksum verification failed"
        fi
        ok "Checksum verified for ${TARBALL}"
    fi
else
    warn "checksum file ${CHECKSUMS_FILE} not found for ${VERSION}; proceeding without checksum verification"
fi

tar -xzf "${TMP}/${TARBALL}" -C "$TMP" || err "extraction failed"

# --- Install binary ---
mkdir -p "$INSTALL_DIR"
install -m755 "${TMP}/sentinel" "${INSTALL_DIR}/sentinel"
ok "Installed sentinel to ${INSTALL_DIR}/sentinel"

# --- Install systemd service (Linux only) ---
if [ "$OS" = "linux" ]; then
    EXEC_START="${INSTALL_DIR}/sentinel"
    if ! command -v systemctl >/dev/null 2>&1; then
        warn "systemctl not found; skipping service installation"
        if [ "$AUTOUPDATE_ENABLED" -eq 1 ]; then
            warn "autoupdate requires systemctl on Linux; skipping autoupdate setup"
        fi
    elif [ "$IS_ROOT" -eq 1 ]; then
        info "Running as root: installing systemd system service..."
        if "${INSTALL_DIR}/sentinel" service install --exec "${EXEC_START}" --enable=true --start=true; then
            echo ""
            ok "systemd system service installed and started."
            printf "\n${BOLD}  Service unit:${RESET}      sentinel\n"
            printf "${BOLD}  Status:${RESET}            systemctl status sentinel\n"
            printf "${BOLD}  Logs:${RESET}              journalctl -u sentinel -f\n"
            printf "${BOLD}  Enable on boot:${RESET}    systemctl enable sentinel\n"

            if [ "$AUTOUPDATE_ENABLED" -eq 1 ]; then
                info "Enabling daily autoupdate timer (system scope)..."
                if "${INSTALL_DIR}/sentinel" service autoupdate install --exec "${EXEC_START}" --enable=true --start=true --service sentinel --scope system; then
                    ok "Autoupdate timer enabled"
                else
                    warn "failed to enable autoupdate timer"
                    warn "you can retry with: sentinel service autoupdate install --scope system"
                fi
            fi
        else
            warn "failed to install/start system service"
            warn "you can retry with: sentinel service install --exec \"${EXEC_START}\""
        fi
    else
        info "Installing systemd user service..."
        if "${INSTALL_DIR}/sentinel" service install --exec "${EXEC_START}" --enable=true --start=true; then
            echo ""
            ok "systemd user service installed and started."

            if [ "$AUTOUPDATE_ENABLED" -eq 1 ]; then
                info "Enabling daily autoupdate timer..."
                if "${INSTALL_DIR}/sentinel" service autoupdate install --exec "${EXEC_START}" --enable=true --start=true --service sentinel --scope user; then
                    ok "Autoupdate timer enabled"
                else
                    warn "failed to enable autoupdate timer"
                    warn "you can retry with: sentinel service autoupdate install"
                fi
            fi

            printf "\n${BOLD}  Enable on login:${RESET}   systemctl --user enable sentinel\n"
            printf "${BOLD}  View logs:${RESET}         journalctl --user -u sentinel -f\n"
            printf "${BOLD}  Auto-update status:${RESET} sentinel service autoupdate status\n"
            printf "\n  ${CYAN}Optional (start at boot without login):${RESET}\n"
            printf "    sudo loginctl enable-linger \$USER\n"
        else
            warn "failed to install/start user service"
            warn "you can retry with: sentinel service install --exec \"${EXEC_START}\""
            warn "if no active user bus is available, login to the target user session and retry"
        fi
    fi
fi

# --- macOS launchd service ---
if [ "$OS" = "darwin" ]; then
    echo ""
    LAUNCHD_SCOPE_LABEL="user"
    LAUNCHD_LOG_PATH="~/.sentinel/logs/sentinel.out.log"
    if [ "$IS_ROOT" -eq 1 ]; then
        LAUNCHD_SCOPE_LABEL="system"
        LAUNCHD_LOG_PATH="/var/log/sentinel/sentinel.out.log"
    fi
    info "Installing launchd ${LAUNCHD_SCOPE_LABEL} service..."
    if "${INSTALL_DIR}/sentinel" service install --exec "${INSTALL_DIR}/sentinel" --enable=true --start=true; then
        ok "launchd ${LAUNCHD_SCOPE_LABEL} service installed and started."
    else
        warn "failed to install/start launchd service"
        warn "you can retry with: sentinel service install"
    fi

    if [ "$AUTOUPDATE_ENABLED" -eq 1 ]; then
        info "Enabling daily autoupdate with launchd (${LAUNCHD_SCOPE_LABEL} scope)..."
        if "${INSTALL_DIR}/sentinel" service autoupdate install --exec "${INSTALL_DIR}/sentinel" --enable=true --start=true --service io.opusdomini.sentinel --scope launchd --on-calendar daily; then
            ok "launchd autoupdate enabled"
        else
            warn "failed to enable launchd autoupdate"
            warn "you can retry with: sentinel service autoupdate install --scope launchd"
        fi
    fi

    printf "\n${BOLD}  Service status:${RESET}    sentinel service status\n"
    printf "${BOLD}  Service logs:${RESET}      tail -f %s\n" "${LAUNCHD_LOG_PATH}"
    printf "${BOLD}  Auto-update status:${RESET} sentinel service autoupdate status\n"
fi

# --- Verify PATH ---
echo ""
if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "sentinel installed successfully"
else
    printf "${BOLD}NOTE:${RESET} %s is not in your PATH.\n" "$INSTALL_DIR"
    printf "Add it:  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
fi
