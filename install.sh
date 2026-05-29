#!/usr/bin/env bash
set -euo pipefail

# Sentinel installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
#
# The installer downloads a published GitHub release archive, verifies its
# checksum when possible, installs the binary, installs shell completion through
# `sentinel completion install --shell auto`, and optionally installs the host
# service. It never edits shell rc files.
#
# Environment variables:
#   REPO               GitHub repository to install from (default: opus-domini/sentinel)
#   INSTALL_DIR        Binary install directory (default: ~/.local/bin, or /usr/local/bin as root)
#   VERSION            Specific version to install, with or without "v" (default: latest)
#   INSTALL_SERVICE    Set to 0/false/no/off to skip service installation
#   ENABLE_AUTOUPDATE  Set to 1/true/yes/on to install and enable daily autoupdate

# --- Configuration ----------------------------------------------------------

APP="sentinel"
PROJECT="Sentinel"
REPO="${REPO:-opus-domini/sentinel}"
INSTALL_SERVICE="${INSTALL_SERVICE:-1}"
ENABLE_AUTOUPDATE="${ENABLE_AUTOUPDATE:-0}"
IS_ROOT=0
if [ "$(id -u)" -eq 0 ]; then
  IS_ROOT=1
fi

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ "$IS_ROOT" -eq 1 ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME:?HOME is required}/.local/bin"
  fi
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
    warn "systemctl not found; skipping service installation"
    if is_true "$ENABLE_AUTOUPDATE"; then
      warn "autoupdate requires systemctl on Linux; skipping autoupdate setup"
    fi
    return 0
  fi

  if [ "$IS_ROOT" -eq 1 ]; then
    info "Installing systemd system service..."
    if "$TARGET" service install --exec "$TARGET" --enable=true --start=true; then
      ok "systemd system service installed and started"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (system scope)..."
        "$TARGET" service autoupdate install --exec "$TARGET" --enable=true --start=true --service sentinel --scope system \
          && ok "Autoupdate timer enabled" \
          || warn "failed to enable autoupdate timer; retry with: sentinel service autoupdate install --scope system"
      fi
    else
      warn "service installation failed; retry with: ${TARGET} service install --exec ${TARGET}"
    fi
  else
    info "Installing systemd user service..."
    if "$TARGET" service install --exec "$TARGET" --enable=true --start=true; then
      ok "systemd user service installed and restarted"
      if is_true "$ENABLE_AUTOUPDATE"; then
        info "Enabling daily autoupdate timer (user scope)..."
        "$TARGET" service autoupdate install --exec "$TARGET" --enable=true --start=true --service sentinel --scope user \
          && ok "Autoupdate timer enabled" \
          || warn "failed to enable autoupdate timer; retry with: sentinel service autoupdate install"
      fi
    else
      warn "service installation failed; retry with: ${TARGET} service install --exec ${TARGET}"
      warn "if no active user bus is available, login to the target user session and retry"
    fi
  fi
}

install_launchd_service() {
  local scope_label="user"
  local log_path="~/.sentinel/logs/sentinel.out.log"

  if [ "$IS_ROOT" -eq 1 ]; then
    scope_label="system"
    log_path="/var/log/sentinel/sentinel.out.log"
  fi

  info "Installing launchd ${scope_label} service..."
  if "$TARGET" service install --exec "$TARGET" --enable=true --start=true; then
    ok "launchd ${scope_label} service installed and started"
    if is_true "$ENABLE_AUTOUPDATE"; then
      info "Enabling daily autoupdate with launchd (${scope_label} scope)..."
      "$TARGET" service autoupdate install --exec "$TARGET" --enable=true --start=true --service io.opusdomini.sentinel --scope launchd --on-calendar daily \
        && ok "launchd autoupdate enabled" \
        || warn "failed to enable launchd autoupdate; retry with: sentinel service autoupdate install --scope launchd"
    fi
    info "Service logs: tail -f ${log_path}"
  else
    warn "service installation failed; retry with: ${TARGET} service install --exec ${TARGET}"
  fi
}

# --- Platform detection -----------------------------------------------------

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) fail "unsupported OS: $OS" ;;
esac

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
  VERSION=$(curl -fsSL --retry 3 --retry-delay 2 "https://api.github.com/repos/${REPO}/releases/latest" \
    | awk -F'"' '/"tag_name"/ { print $4; exit }')
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
    warn "${CHECKSUMS_FILE} not found for ${VERSION}; proceeding without checksum verification"
  fi
else
  warn "no checksum tool found (sha256sum/shasum); release integrity verification will be skipped"
fi

tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP" || fail "extraction failed"
[ -x "${TMP}/${APP}" ] || fail "archive did not contain an executable ${APP} binary"

# --- Binary installation ----------------------------------------------------

mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/${APP}"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "${TMP}/${APP}" "$TARGET"
else
  cp "${TMP}/${APP}" "$TARGET"
  chmod 0755 "$TARGET"
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
