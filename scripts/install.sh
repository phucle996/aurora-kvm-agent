#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_NAME="install.sh"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

APP_NAME="${AURORA_AGENT_BIN_NAME:-aurora-kvm-agent}"
INSTALL_DIR="${AURORA_AGENT_INSTALL_DIR:-/usr/local/bin}"
BIN_PATH="${INSTALL_DIR}/${APP_NAME}"
SERVICE_NAME="${AURORA_AGENT_SERVICE_NAME:-aurora-kvm-agent.service}"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
ENV_FILE="${AURORA_AGENT_ENV_FILE:-/etc/aurora-kvm-agent.env}"
VERSION="${AURORA_AGENT_VERSION:-latest}"

log() {
  printf '[%s] %s\n' "$SCRIPT_NAME" "$1"
}

warn() {
  printf '[%s][warn] %s\n' "$SCRIPT_NAME" "$1" >&2
}

trap 'rc=$?; line="${BASH_LINENO[0]:-$LINENO}"; cmd="${BASH_COMMAND:-unknown}"; printf "[%s][error] rc=%s line=%s cmd=%s\n" "$SCRIPT_NAME" "$rc" "$line" "$cmd" >&2' ERR

run_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    if [ -n "${SUDO_PASSWORD_B64:-}" ]; then
      local sudo_password
      sudo_password="$(printf '%s' "$SUDO_PASSWORD_B64" | base64 -d 2>/dev/null || true)"
      if [ -n "$sudo_password" ]; then
        printf '%s\n' "$sudo_password" | sudo -S -p '' "$@"
        return
      fi
    fi
    sudo "$@"
    return
  fi
  echo "This installer requires root or sudo." >&2
  exit 1
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: ${cmd}" >&2
    exit 1
  fi
}

resolve_repo_default() {
  local env_repo="${AURORA_AGENT_GITHUB_REPO:-}"
  if [ -n "$env_repo" ]; then
    printf '%s' "$env_repo"
    return
  fi

  if command -v git >/dev/null 2>&1 && [ -d "${ROOT_DIR}/.git" ]; then
    local remote
    remote="$(git -C "$ROOT_DIR" config --get remote.origin.url 2>/dev/null || true)"
    if [ -n "$remote" ]; then
      remote="${remote#git@github.com:}"
      remote="${remote#https://github.com/}"
      remote="${remote%.git}"
      if [ -n "$remote" ] && printf '%s' "$remote" | grep -q '/'; then
        printf '%s' "$remote"
        return
      fi
    fi
  fi

  # Fallback default; override via AURORA_AGENT_GITHUB_REPO if needed.
  printf 'phucle996/aurora-kvm-agent'
}

resolve_arch() {
  case "$(uname -m 2>/dev/null || true)" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *)
      echo "Unsupported architecture: $(uname -m 2>/dev/null || echo unknown)" >&2
      exit 1
      ;;
  esac
}

download_file() {
  local url="$1"
  local dst="$2"
  local token="${AURORA_AGENT_GITHUB_TOKEN:-}"

  if command -v curl >/dev/null 2>&1; then
    if [ -n "$token" ]; then
      curl -fL --retry 3 --retry-delay 2 --connect-timeout 10 \
        -H "Authorization: Bearer ${token}" \
        -o "$dst" "$url"
    else
      curl -fL --retry 3 --retry-delay 2 --connect-timeout 10 \
        -o "$dst" "$url"
    fi
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    if [ -n "$token" ]; then
      wget --tries=3 --timeout=10 \
        --header="Authorization: Bearer ${token}" \
        -O "$dst" "$url"
    else
      wget --tries=3 --timeout=10 -O "$dst" "$url"
    fi
    return
  fi

  echo "curl/wget not available for download" >&2
  exit 1
}

verify_checksum() {
  local file="$1"
  local expected="$2"

  if command -v sha256sum >/dev/null 2>&1; then
    local actual
    actual="$(sha256sum "$file" | awk '{print $1}')"
    [ "$actual" = "$expected" ]
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    local actual
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    [ "$actual" = "$expected" ]
    return
  fi

  warn "sha256 tool not found (sha256sum/shasum); skipping checksum verification"
  return 0
}

install_systemd_unit() {
  local local_unit="${ROOT_DIR}/systemd/aurora-kvm-agent.service"
  if [ -f "$local_unit" ]; then
    run_root install -m 0644 "$local_unit" "$SERVICE_PATH"
    return
  fi

  run_root bash -lc "cat > '${SERVICE_PATH}' <<'EOF'
[Unit]
Description=Aurora KVM Metrics Agent
After=network-online.target libvirtd.service
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
EnvironmentFile=-/etc/aurora-kvm-agent.env
ExecStart=/usr/local/bin/aurora-kvm-agent
Restart=always
RestartSec=3
KillSignal=SIGTERM
TimeoutStopSec=35
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF"
}

ensure_env_file() {
  if run_root test -f "$ENV_FILE"; then
    return
  fi

  run_root bash -lc "cat > '${ENV_FILE}' <<'EOF'
AURORA_NODE_ID=
AURORA_LIBVIRT_URI=qemu+unix:///system
AURORA_STREAM_MODE=grpc
AURORA_BACKEND_GRPC_ADDR=127.0.0.1:8443
AURORA_BACKEND_TOKEN=
AURORA_TLS_ENABLED=false
AURORA_TLS_SKIP_VERIFY=false
AURORA_LOG_JSON=true
AURORA_LOG_LEVEL=info
AURORA_NODE_POLL_INTERVAL=3s
AURORA_VM_POLL_INTERVAL=1s
AURORA_SHUTDOWN_TIMEOUT=20s
EOF"
}

main() {
  require_cmd tar
  local repo
  repo="$(resolve_repo_default)"
  local arch
  arch="$(resolve_arch)"

  local asset="${APP_NAME}_linux_${arch}.tar.gz"
  local checksum_asset="checksums.txt"

  local base_url
  if [ "$VERSION" = "latest" ]; then
    base_url="https://github.com/${repo}/releases/latest/download"
  else
    base_url="https://github.com/${repo}/releases/download/${VERSION}"
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d /tmp/${APP_NAME}-install.XXXXXX)"
  cleanup() {
    rm -rf "$tmp_dir" || true
  }
  trap cleanup EXIT

  local tarball="${tmp_dir}/${asset}"
  local checksums="${tmp_dir}/${checksum_asset}"

  log "downloading release asset repo=${repo} version=${VERSION} arch=${arch}"
  download_file "${base_url}/${asset}" "$tarball"
  download_file "${base_url}/${checksum_asset}" "$checksums"

  local expected
  expected="$(awk -v asset="$asset" '{name=$2; sub(/^.*\//, "", name); if(name==asset){print $1; exit}}' "$checksums")"
  if [ -z "$expected" ]; then
    echo "Cannot find checksum for ${asset} in ${checksum_asset}" >&2
    exit 1
  fi

  if ! verify_checksum "$tarball" "$expected"; then
    echo "Checksum verification failed for ${asset}" >&2
    exit 1
  fi
  log "checksum verification passed"

  tar -xzf "$tarball" -C "$tmp_dir"
  local extracted="${tmp_dir}/${APP_NAME}_linux_${arch}"
  if [ ! -f "$extracted" ]; then
    echo "Extracted binary not found: ${extracted}" >&2
    exit 1
  fi

  run_root mkdir -p "$INSTALL_DIR"
  run_root install -m 0755 "$extracted" "$BIN_PATH"
  log "installed binary -> ${BIN_PATH}"

  ensure_env_file
  install_systemd_unit

  if command -v systemctl >/dev/null 2>&1; then
    run_root systemctl daemon-reload
    run_root systemctl enable --now "$SERVICE_NAME"
    run_root systemctl restart "$SERVICE_NAME"
    if run_root systemctl is-active --quiet "$SERVICE_NAME"; then
      log "service is active: ${SERVICE_NAME}"
    else
      warn "service is not active: ${SERVICE_NAME}"
      run_root systemctl status "$SERVICE_NAME" --no-pager || true
      exit 1
    fi
  else
    warn "systemctl not found; service was not started"
  fi

  log "installation completed"
}

main "$@"
