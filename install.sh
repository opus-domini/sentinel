#!/usr/bin/env bash
set -euo pipefail

# Sentinel installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
#
# The installer downloads a published GitHub release archive, verifies its
# checksum, installs the binary, installs shell completion through
# `sentinel completion install --shell auto`, and optionally installs the host
# service. It never edits shell rc files.
#
# Environment variables:
#   REPO               GitHub repository to install from (default: opus-domini/sentinel)
#   INSTALL_DIR        Binary install directory (default: ~/.local/bin, or /usr/local/bin as root)
#   VERSION            Specific version to install, with or without "v" (default: latest)
#   INSTALL_SERVICE    Set to 0/false/no/off to skip service installation
#   INSTALL_SCOPE      Installation scope: auto, user, or system (default: auto)
#   ENABLE_AUTOUPDATE  Set to 1/true/yes/on to install and enable daily autoupdate

# --- Configuration ----------------------------------------------------------

APP="sentinel"
PROJECT="Sentinel"
REPO="${REPO:-opus-domini/sentinel}"
INSTALL_SERVICE="${INSTALL_SERVICE:-1}"
INSTALL_SCOPE="${INSTALL_SCOPE:-auto}"
ENABLE_AUTOUPDATE="${ENABLE_AUTOUPDATE:-0}"
IS_ROOT=0
if [ "$(id -u)" -eq 0 ]; then
  IS_ROOT=1
fi

# --- Output helpers ---------------------------------------------------------

if [ -t 1 ]; then
  BOLD='\033[1m'
  RED='\033[0;31m'
  YELLOW='\033[0;33m'
  GREEN='\033[0;32m'
  CYAN='\033[0;36m'
  RESET='\033[0m'
else
  BOLD=''
  RED=''
  YELLOW=''
  GREEN=''
  CYAN=''
  RESET=''
fi

info() { printf '%b%s%b\n' "$CYAN" "$*" "$RESET"; }
ok() { printf '%b%s%b\n' "$GREEN" "$*" "$RESET"; }
warn() { printf '%bwarning: %s%b\n' "$YELLOW" "$*" "$RESET" >&2; }
fail() { printf '%berror: %s%b\n' "$RED" "$*" "$RESET" >&2; exit 1; }

important() {
  printf '\n%b==================== IMPORTANT ====================%b\n' "$YELLOW$BOLD" "$RESET" >&2
  printf '%b%s%b\n' "$YELLOW$BOLD" "$*" "$RESET" >&2
  printf '%b===================================================%b\n\n' "$YELLOW$BOLD" "$RESET" >&2
}

# --- Common helpers ---------------------------------------------------------

need() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|True|yes|YES|Yes|on|ON|On) return 0 ;;
    *) return 1 ;;
  esac
}

is_false() {
  case "${1:-}" in
    0|false|FALSE|False|no|NO|No|off|OFF|Off) return 0 ;;
    *) return 1 ;;
  esac
}

normalize_version() {
  if [ "${1#v}" = "$1" ]; then
    printf 'v%s\n' "$1"
  else
    printf '%s\n' "$1"
  fi
}

checksum_tool() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf 'sha256sum\n'
  elif command -v shasum >/dev/null 2>&1; then
    printf 'shasum\n'
  else
    return 1
  fi
}

install_systemd_service() {
  if ! command -v systemctl >/dev/null 2>&1; then
    fail "systemctl not found; set INSTALL_SERVICE=0 for a standalone installation"
  fi

  if [ "$RESOLVED_SCOPE" = "system" ]; then
    info "Installing systemd system service..."
    if "$TARGET" service install --scope system --exec "$TARGET" --enable=true --start=true; then
      ok "systemd system service installed and started"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (system scope)..."
        "$TARGET" service autoupdate install --enable=true --start=true --scope system \
          && ok "Autoupdate timer enabled" \
          || rollback_install "failed to enable the system autoupdate timer"
      fi
    else
      rollback_install "system service installation failed"
    fi
  else
    info "Installing systemd user service..."
    if "$TARGET" service install --scope user --exec "$TARGET" --enable=true --start=true; then
      ok "systemd user service installed and restarted"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (user scope)..."
        "$TARGET" service autoupdate install --enable=true --start=true --scope user \
          && ok "Autoupdate timer enabled" \
          || rollback_install "failed to enable the user autoupdate timer"
      fi
    else
      rollback_install "user service installation failed; ensure the target user has an active systemd session"
    fi
  fi
}

install_launchd_service() {
  local scope_label="user"
  local log_path="~/.sentinel/logs/sentinel.out.log"

  if [ "$RESOLVED_SCOPE" = "system" ]; then
    scope_label="system"
    log_path="/var/log/sentinel/sentinel.out.log"
  fi

  info "Installing launchd ${scope_label} service..."
  if "$TARGET" service install --scope "$RESOLVED_SCOPE" --exec "$TARGET" --enable=true --start=true; then
    ok "launchd ${scope_label} service installed and started"
    if is_true "$ENABLE_AUTOUPDATE"; then
      info "Enabling daily autoupdate with launchd (${scope_label} scope)..."
      "$TARGET" service autoupdate install --enable=true --start=true --scope "$RESOLVED_SCOPE" --on-calendar daily \
        && ok "launchd autoupdate enabled" \
        || rollback_install "failed to enable launchd autoupdate"
    fi
    info "Service logs: tail -f ${log_path}"
  else
    rollback_install "launchd service installation failed"
  fi
}

# --- Platform detection -----------------------------------------------------

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) fail "unsupported OS: $OS" ;;
esac

# --- Deployment scope preflight ---------------------------------------------

case "$INSTALL_SCOPE" in
  auto|user|system) ;;
  *) fail "invalid INSTALL_SCOPE=${INSTALL_SCOPE}; expected auto, user, or system" ;;
esac

USER_HOME="${HOME:?HOME is required}"
if [ "$IS_ROOT" -eq 1 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ] && command -v getent >/dev/null 2>&1; then
  SUDO_HOME=$(getent passwd "$SUDO_USER" | awk -F: '{print $6}')
  if [ -n "$SUDO_HOME" ]; then
    USER_HOME="$SUDO_HOME"
  fi
fi

if [ "$OS" = "linux" ]; then
  USER_SERVICE_PATH="${USER_HOME}/.config/systemd/user/sentinel.service"
  SYSTEM_SERVICE_PATH="/etc/systemd/system/sentinel.service"
else
  USER_SERVICE_PATH="${USER_HOME}/Library/LaunchAgents/io.opusdomini.sentinel.plist"
  SYSTEM_SERVICE_PATH="/Library/LaunchDaemons/io.opusdomini.sentinel.plist"
fi

HAS_USER_SERVICE=0
HAS_SYSTEM_SERVICE=0
[ -f "$USER_SERVICE_PATH" ] && HAS_USER_SERVICE=1
[ -f "$SYSTEM_SERVICE_PATH" ] && HAS_SYSTEM_SERVICE=1

if [ "$HAS_USER_SERVICE" -eq 1 ] && [ "$HAS_SYSTEM_SERVICE" -eq 1 ]; then
  fail "Sentinel is installed in both user and system scope; remove one deployment before installing"
fi

EXISTING_SCOPE=""
[ "$HAS_USER_SERVICE" -eq 1 ] && EXISTING_SCOPE="user"
[ "$HAS_SYSTEM_SERVICE" -eq 1 ] && EXISTING_SCOPE="system"

RESOLVED_SCOPE="$INSTALL_SCOPE"
if [ "$RESOLVED_SCOPE" = "auto" ]; then
  if [ -n "$EXISTING_SCOPE" ]; then
    RESOLVED_SCOPE="$EXISTING_SCOPE"
  elif [ "$IS_ROOT" -eq 1 ]; then
    RESOLVED_SCOPE="system"
  else
    RESOLVED_SCOPE="user"
  fi
fi

if [ -n "$EXISTING_SCOPE" ] && [ "$EXISTING_SCOPE" != "$RESOLVED_SCOPE" ]; then
  fail "Sentinel is already installed in ${EXISTING_SCOPE} scope; uninstall it before installing in ${RESOLVED_SCOPE} scope"
fi
if [ "$RESOLVED_SCOPE" = "system" ] && [ "$IS_ROOT" -ne 1 ]; then
  fail "Sentinel is installed system-wide; re-run the installer as root with INSTALL_SCOPE=system"
fi
if [ "$RESOLVED_SCOPE" = "user" ] && [ "$IS_ROOT" -eq 1 ]; then
  fail "Sentinel is installed for ${SUDO_USER:-a user}; run the installer as that user without sudo"
fi

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ "$RESOLVED_SCOPE" = "system" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${USER_HOME}/.local/bin"
  fi
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l|armv7*|armhf|arm)
    if [ "$OS" = "linux" ]; then
      ARCH="arm"
    else
      fail "unsupported architecture on ${OS}: $(uname -m)"
    fi
    ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

# --- Dependency checks ------------------------------------------------------

need awk
need curl
need tar

if ! command -v tmux >/dev/null 2>&1; then
  important "tmux was not found on this host. ${PROJECT} installed successfully, but tmux features stay disabled until tmux is installed."
fi

# --- Version resolution -----------------------------------------------------

VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  info "Fetching latest ${PROJECT} release..."
  if ! VERSION=$(curl -fsSL --retry 3 --retry-delay 2 "https://api.github.com/repos/${REPO}/releases/latest" \
    | awk -F'"' '
      /"tag_name"/ && tag == "" { tag = $4 }
      END { if (tag != "") print tag }
    '); then
    fail "could not fetch latest release metadata; set VERSION=vX.Y.Z"
  fi
fi
[ -n "$VERSION" ] || fail "could not determine latest release; set VERSION=vX.Y.Z"
VERSION=$(normalize_version "$VERSION")
ASSET_VERSION="${VERSION#v}"

# --- Download and verification ---------------------------------------------

ARCHIVE="${APP}-${ASSET_VERSION}-${OS}-${ARCH}.tar.gz"
CHECKSUMS_FILE="${APP}-${ASSET_VERSION}-checksums.txt"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Installing ${PROJECT} ${VERSION} (${OS}/${ARCH})..."
info "Downloading ${ARCHIVE}..."
curl -fsSL --retry 3 --retry-delay 2 -o "${TMP}/${ARCHIVE}" "${BASE_URL}/${ARCHIVE}" \
  || fail "download failed - check that ${VERSION} exists"

if tool=$(checksum_tool); then
  if curl -fsSL --retry 3 --retry-delay 2 -o "${TMP}/${CHECKSUMS_FILE}" "${BASE_URL}/${CHECKSUMS_FILE}"; then
    TARGET_CHECKSUM="${TMP}/${APP}-target-checksum.txt"
    awk -v target="$ARCHIVE" '
      NF >= 2 {
        file = $NF
        gsub(/^\*/, "", file)
        if (file == target) {
          print $1 "  " target
        }
      }
    ' "${TMP}/${CHECKSUMS_FILE}" > "$TARGET_CHECKSUM"

    [ -s "$TARGET_CHECKSUM" ] || fail "checksum entry for ${ARCHIVE} was not found in ${CHECKSUMS_FILE}"

    info "Verifying release checksum..."
    if [ "$tool" = "sha256sum" ]; then
      (cd "$TMP" && sha256sum -c "$(basename "$TARGET_CHECKSUM")") || fail "checksum verification failed"
    else
      (cd "$TMP" && shasum -a 256 -c "$(basename "$TARGET_CHECKSUM")") || fail "checksum verification failed"
    fi
    ok "Checksum verified for ${ARCHIVE}"
  else
    fail "${CHECKSUMS_FILE} not found for ${VERSION}; refusing an unverified installation"
  fi
else
  fail "sha256sum or shasum is required to verify the release"
fi

tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP" || fail "extraction failed"
[ -x "${TMP}/${APP}" ] || fail "archive did not contain an executable ${APP} binary"

if [ "$RESOLVED_SCOPE" = "system" ]; then
  CONFIG_PATH="/etc/sentinel/config.toml"
  DATA_DIR="/var/lib/sentinel"
  if [ ! -f "$CONFIG_PATH" ] && [ -f "/root/.sentinel/config.toml" ]; then
    CONFIG_PATH="/root/.sentinel/config.toml"
    DATA_DIR="/root/.sentinel"
  fi
else
  CONFIG_PATH="${USER_HOME}/.sentinel/config.toml"
  DATA_DIR="${USER_HOME}/.sentinel"
fi

SENTINEL_DATA_DIR="$DATA_DIR" "${TMP}/${APP}" --config "$CONFIG_PATH" config validate --effective \
  || fail "the downloaded Sentinel version rejected ${CONFIG_PATH}"

# --- Binary installation ----------------------------------------------------

mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/${APP}"
SENTINEL_DATA_DIR="$DATA_DIR" "${TMP}/${APP}" --config "$CONFIG_PATH" service install --check --scope "$RESOLVED_SCOPE" --exec "$TARGET" \
  || fail "installation preflight failed before replacing ${TARGET}"
PREVIOUS_BINARY=""
if [ -f "$TARGET" ]; then
  PREVIOUS_BINARY="${TMP}/${APP}.previous"
  cp -p "$TARGET" "$PREVIOUS_BINARY"
fi

rollback_install() {
  local reason="$1"
  if [ -n "$PREVIOUS_BINARY" ] && [ -f "$PREVIOUS_BINARY" ]; then
    if ! cp -p "$PREVIOUS_BINARY" "$TARGET"; then
      fail "$reason; rollback also failed to restore the previous binary at ${TARGET}"
    fi
    if "$TARGET" service restart --scope "$RESOLVED_SCOPE" >/dev/null 2>&1; then
      fail "$reason; the previous binary was restored and restarted"
    fi
    fail "$reason; the previous binary was restored, but its service could not be restarted"
  fi
  "$TARGET" service uninstall --scope "$RESOLVED_SCOPE" >/dev/null 2>&1 || true
  rm -f "$TARGET" || fail "$reason; cleanup also failed to remove ${TARGET}"
  fail "$reason; the incomplete binary installation was removed"
}

if command -v install >/dev/null 2>&1; then
  install -m 0755 "${TMP}/${APP}" "$TARGET" || rollback_install "binary installation failed"
else
  cp "${TMP}/${APP}" "$TARGET" || rollback_install "binary installation failed"
  chmod 0755 "$TARGET" || rollback_install "setting binary permissions failed"
fi
ok "Installed ${PROJECT} to ${TARGET}"

# --- Completion installation ------------------------------------------------

if "$TARGET" completion install --shell auto; then
  ok "Shell completion installed for the detected shell"
else
  warn "could not install shell completion"
fi

# --- Service installation ---------------------------------------------------

if is_false "$INSTALL_SERVICE"; then
  info "Skipping service installation because INSTALL_SERVICE=${INSTALL_SERVICE}"
elif [ "$OS" = "linux" ]; then
  install_systemd_service
else
  install_launchd_service
fi

# --- PATH check -------------------------------------------------------------

case ":${PATH:-}:" in
  *":${INSTALL_DIR}:"*) ok "${APP} is available on PATH" ;;
  *) warn "${INSTALL_DIR} is not on PATH; add: export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
esac

# --- Final guidance ---------------------------------------------------------

cat <<EOF

${BOLD}${PROJECT} installed:${RESET}
  binary:  ${TARGET}
  service: ${APP}

Next steps:
  ${APP} service status
  ${APP} doctor
  Open http://127.0.0.1:4040 when the service is running.
EOF
