# Aurora KVM Agent

Production-style KVM node agent written in Go.

Agent này chạy trên hypervisor host để:

- Thu thập realtime metrics của node KVM
- Thu thập realtime metrics per-VM từ libvirt
- Stream dữ liệu về control plane qua `gRPC` hoặc `WebSocket`
- Tự reconnect khi mất kết nối libvirt/backend
- Chạy nền bằng `systemd`

## Features

- Libvirt integration bằng `go-libvirt`
- Scheduler tách vòng poll VM và Node
- Stream abstraction (`grpc` / `websocket`)
- TLS/mTLS support cho backend stream
- Structured logging (`json` hoặc `text`)

## Repository Structure

```text
cmd/agent/main.go                 # entrypoint
internal/config                   # env config + validation
internal/libvirt                  # libvirt connection + metrics readers
internal/collector                # scheduler + collectors
internal/stream                   # grpc/ws clients + encoder
internal/agent                    # lifecycle + health + shutdown
internal/system                   # host counters from /proc
proto/metrics.proto               # streaming contracts
scripts/install.sh                # install binary + systemd
systemd/aurora-kvm-agent.service  # service unit
```

## Requirements

- Go `1.24+`
- Linux host có KVM/libvirt
- Quyền đọc libvirt socket (`qemu+unix:///system` mặc định)

## Quick Start (Local)

```bash
cd vm-agent
go mod tidy
go test ./...
go run ./cmd/agent
```

## Install as Service

```bash
cd vm-agent
chmod +x scripts/install.sh
./scripts/install.sh
```

Script sẽ:

- build binary `aurora-kvm-agent`
- install vào `/usr/local/bin/aurora-kvm-agent`
- copy unit file vào `/etc/systemd/system/aurora-kvm-agent.service`
- tạo file env `/etc/aurora-kvm-agent.env` (nếu chưa có)
- `systemctl enable --now aurora-kvm-agent.service`

## Runtime Flow

```text
connect libvirt
 -> poll VM metrics (1s)
 -> poll node metrics (3s)
 -> encode frame
 -> stream to backend
 -> health loop / reconnect
```

## Main Environment Variables

Chi tiết đầy đủ xem `CONFIGURATION.md`.

- `AURORA_NODE_ID`
- `AURORA_LIBVIRT_URI`
- `AURORA_STREAM_MODE` (`grpc` | `websocket`)
- `AURORA_BACKEND_GRPC_ADDR`
- `AURORA_BACKEND_WS_URL`
- `AURORA_TLS_ENABLED`
- `AURORA_TLS_CA_PATH`, `AURORA_TLS_CERT_PATH`, `AURORA_TLS_KEY_PATH`

## Notes

- gRPC streaming hiện dùng JSON codec (không phụ thuộc generated pb trong runtime path).
- `proto/metrics.proto` đã sẵn sàng để generate client/server typed contract sau này.

## License

Chưa khai báo license file. Bạn nên thêm `LICENSE` trước khi public repo.
