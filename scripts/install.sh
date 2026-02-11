#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="/usr/local/bin"
SERVICE_NAME="aurora-kvm-agent.service"

run_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  echo "This installer requires root or sudo." >&2
  exit 1
}

echo "[install] building aurora-kvm-agent"
cd "$ROOT_DIR"
go build -o aurora-kvm-agent ./cmd/agent

echo "[install] installing binary -> ${BIN_DIR}/aurora-kvm-agent"
run_root install -m 0755 aurora-kvm-agent "${BIN_DIR}/aurora-kvm-agent"

echo "[install] installing systemd unit"
run_root install -m 0644 "${ROOT_DIR}/systemd/${SERVICE_NAME}" "/etc/systemd/system/${SERVICE_NAME}"

if [ ! -f /etc/aurora-kvm-agent.env ]; then
  echo "[install] creating /etc/aurora-kvm-agent.env"
  run_root tee /etc/aurora-kvm-agent.env >/dev/null <<'ENVEOF'
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
ENVEOF
fi

echo "[install] reload and enable systemd service"
run_root systemctl daemon-reload
run_root systemctl enable --now "$SERVICE_NAME"

echo "[install] done"
