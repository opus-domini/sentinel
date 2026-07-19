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
#   INSTALL_SCOPE      Installation scope: user or system. Existing installs are
#                      detected automatically; fresh non-interactive installs
#                      must set this explicitly.
#   ENABLE_AUTOUPDATE  Set to 1/true/yes/on to install and enable daily autoupdate

# --- Configuration ----------------------------------------------------------

APP="sentinel"
PROJECT="Sentinel"
REPO="${REPO:-opus-domini/sentinel}"
INSTALL_SERVICE="${INSTALL_SERVICE:-1}"
INSTALL_SCOPE="${INSTALL_SCOPE:-auto}"
ENABLE_AUTOUPDATE="${ENABLE_AUTOUPDATE:-0}"
INSTALL_SOURCE="${SENTINEL_INSTALL_SOURCE:-}"
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

run_for_scope() {
  if [ "$RESOLVED_SCOPE" = "system" ] && [ "$IS_ROOT" -ne 1 ]; then
    sudo "$@"
  else
    "$@"
  fi
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
    if run_for_scope "$TARGET" service install --scope system --exec "$TARGET" --enable=true --start=true; then
      ok "systemd system service installed and started"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (system scope)..."
        run_for_scope "$TARGET" service autoupdate install --enable=true --start=true --scope system \
          && ok "Autoupdate timer enabled" \
          || rollback_install "failed to enable the system autoupdate timer"
      fi
    else
      rollback_install "system service installation failed"
    fi
  else
    info "Installing systemd user service..."
    if run_for_scope "$TARGET" service install --scope user --exec "$TARGET" --enable=true --start=true; then
      ok "systemd user service installed and restarted"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (user scope)..."
        run_for_scope "$TARGET" service autoupdate install --enable=true --start=true --scope user \
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
  if run_for_scope "$TARGET" service install --scope "$RESOLVED_SCOPE" --exec "$TARGET" --enable=true --start=true; then
    ok "launchd ${scope_label} service installed and started"
    if is_true "$ENABLE_AUTOUPDATE"; then
      info "Enabling daily autoupdate with launchd (${scope_label} scope)..."
      run_for_scope "$TARGET" service autoupdate install --enable=true --start=true --scope "$RESOLVED_SCOPE" --on-calendar daily \
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
if [ -z "$INSTALL_SOURCE" ]; then
  need curl
  need tar
fi

if ! command -v tmux >/dev/null 2>&1; then
  important "tmux was not found on this host. ${PROJECT} installed successfully, but tmux features stay disabled until tmux is installed."
fi

# --- Version resolution -----------------------------------------------------

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

if [ -n "$INSTALL_SOURCE" ]; then
  [ -x "$INSTALL_SOURCE" ] || fail "local install source is not executable: ${INSTALL_SOURCE}"
  CANDIDATE="$INSTALL_SOURCE"
  info "Installing locally built ${PROJECT} (${OS}/${ARCH})..."
else
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
  ARCHIVE="${APP}-${ASSET_VERSION}-${OS}-${ARCH}.tar.gz"
  CHECKSUMS_FILE="${APP}-${ASSET_VERSION}-checksums.txt"
  BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

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
  CANDIDATE="${TMP}/${APP}"
fi

# --- Deployment scope resolution -------------------------------------------

if { exec 3<>/dev/tty; } 2>/dev/null; then
  RESOLVED_SCOPE=$("$CANDIDATE" service resolve-install-scope --scope "$INSTALL_SCOPE" --interactive <&3) \
    || fail "could not resolve the installation scope"
  exec 3>&-
else
  RESOLVED_SCOPE=$("$CANDIDATE" service resolve-install-scope --scope "$INSTALL_SCOPE") \
    || fail "could not resolve the installation scope"
fi

if [ "$RESOLVED_SCOPE" = "system" ] && [ "$IS_ROOT" -ne 1 ]; then
  need sudo
  info "System scope selected; requesting sudo for system files and service management..."
  sudo -v || fail "system installation requires sudo access"
fi
if [ "$RESOLVED_SCOPE" = "user" ] && [ "$IS_ROOT" -eq 1 ]; then
  fail "user scope must be installed by that user; re-run without sudo with INSTALL_SCOPE=user"
fi

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ "$RESOLVED_SCOPE" = "system" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${USER_HOME}/.local/bin"
  fi
fi

if [ "$OS" = "linux" ]; then
  SYSTEM_SERVICE_PATH="/etc/systemd/system/sentinel.service"
else
  SYSTEM_SERVICE_PATH="/Library/LaunchDaemons/io.opusdomini.sentinel.plist"
fi
HAS_SYSTEM_SERVICE=0
[ -f "$SYSTEM_SERVICE_PATH" ] && HAS_SYSTEM_SERVICE=1

if [ "$RESOLVED_SCOPE" = "system" ]; then
  if [ "$OS" = "linux" ]; then
    CONFIG_PATH="/etc/sentinel/config.toml"
  else
    CONFIG_PATH="/Library/Preferences/io.opusdomini.sentinel.toml"
  fi
else
  CONFIG_PATH="${USER_HOME}/.sentinel/config.toml"
fi

if [ "$RESOLVED_SCOPE" = "system" ] && [ "$HAS_SYSTEM_SERVICE" -eq 1 ]; then
  info "Checking the system deployment filesystem layout..."
  run_for_scope "$CANDIDATE" service migrate --scope system \
    || fail "system deployment migration failed before replacing the binary"
fi

if [ -f "$CONFIG_PATH" ]; then
  run_for_scope "$CANDIDATE" --config "$CONFIG_PATH" config validate \
    || fail "the downloaded Sentinel version rejected ${CONFIG_PATH}"
fi

# --- Binary installation ----------------------------------------------------

TARGET="${INSTALL_DIR}/${APP}"
run_for_scope mkdir -p "$INSTALL_DIR"
run_for_scope "$CANDIDATE" service install --check --scope "$RESOLVED_SCOPE" --exec "$TARGET" \
  || fail "installation preflight failed before replacing ${TARGET}"
PREVIOUS_BINARY=""
if [ -f "$TARGET" ]; then
  PREVIOUS_BINARY="${TMP}/${APP}.previous"
  cp -p "$TARGET" "$PREVIOUS_BINARY"
fi

rollback_install() {
  local reason="$1"
  if [ -n "$PREVIOUS_BINARY" ] && [ -f "$PREVIOUS_BINARY" ]; then
    if ! run_for_scope cp -p "$PREVIOUS_BINARY" "$TARGET"; then
      fail "$reason; rollback also failed to restore the previous binary at ${TARGET}"
    fi
    if run_for_scope "$TARGET" service restart --scope "$RESOLVED_SCOPE" >/dev/null 2>&1; then
      fail "$reason; the previous binary was restored and restarted"
    fi
    fail "$reason; the previous binary was restored, but its service could not be restarted"
  fi
  run_for_scope "$TARGET" service uninstall --scope "$RESOLVED_SCOPE" >/dev/null 2>&1 || true
  run_for_scope rm -f "$TARGET" || fail "$reason; cleanup also failed to remove ${TARGET}"
  fail "$reason; the incomplete binary installation was removed"
}

if command -v install >/dev/null 2>&1; then
  run_for_scope install -m 0755 "$CANDIDATE" "$TARGET" || rollback_install "binary installation failed"
else
  run_for_scope cp "$CANDIDATE" "$TARGET" || rollback_install "binary installation failed"
  run_for_scope chmod 0755 "$TARGET" || rollback_install "setting binary permissions failed"
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

SERVICE_SUMMARY="$APP"
if is_false "$INSTALL_SERVICE"; then
  SERVICE_SUMMARY="not installed (INSTALL_SERVICE=${INSTALL_SERVICE})"
fi
printf '\n%b%s installed:%b\n' "$BOLD" "$PROJECT" "$RESET"
printf '  binary:  %s\n' "$TARGET"
printf '  service: %s\n\n' "$SERVICE_SUMMARY"
printf 'Next steps:\n'
printf '  %s service status\n' "$APP"
if [ "$RESOLVED_SCOPE" = "system" ]; then
  printf '  sudo %s doctor\n' "$APP"
  printf '  sudo %s update apply --scope system\n' "$APP"
else
  printf '  %s doctor\n' "$APP"
  printf '  %s update apply --scope user\n' "$APP"
fi
printf '  Open http://127.0.0.1:4040 when the service is running.\n'
