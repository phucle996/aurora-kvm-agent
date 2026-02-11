# Configuration

Agent đọc config từ environment variables.

## Required / Important

| Variable | Default | Description |
|---|---|---|
| `AURORA_NODE_ID` | hostname | Node identity gửi về control plane |
| `AURORA_LIBVIRT_URI` | `qemu+unix:///system` | Libvirt URI |
| `AURORA_STREAM_MODE` | `grpc` | `grpc` hoặc `websocket` |
| `AURORA_BACKEND_GRPC_ADDR` | `127.0.0.1:8443` | gRPC backend address |
| `AURORA_BACKEND_WS_URL` | `ws://127.0.0.1:8080/ws/metrics` | WS backend URL |

## Polling / Retry

| Variable | Default |
|---|---|
| `AURORA_VM_POLL_INTERVAL` | `1s` |
| `AURORA_NODE_POLL_INTERVAL` | `3s` |
| `AURORA_HEALTH_INTERVAL` | `10s` |
| `AURORA_RECONNECT_INTERVAL` | `4s` |
| `AURORA_SHUTDOWN_TIMEOUT` | `20s` |
| `AURORA_COLLECTOR_ERROR_BACKOFF` | `1500ms` |
| `AURORA_RECONNECT_MAX_JITTER` | `900ms` |

## Stream / Auth

| Variable | Default | Description |
|---|---|---|
| `AURORA_BACKEND_TOKEN` | empty | Bearer token |
| `AURORA_GRPC_NODE_STREAM_METHOD` | `/aurora.metrics.v1.MetricsService/StreamNodeMetrics` | gRPC stream method |
| `AURORA_GRPC_VM_STREAM_METHOD` | `/aurora.metrics.v1.MetricsService/StreamVMMetrics` | gRPC stream method |

## TLS

| Variable | Default | Description |
|---|---|---|
| `AURORA_TLS_ENABLED` | `false` | bật TLS cho backend stream |
| `AURORA_TLS_SKIP_VERIFY` | `false` | skip cert verify (không khuyến nghị production) |
| `AURORA_TLS_CA_PATH` | empty | CA cert path |
| `AURORA_TLS_CERT_PATH` | empty | client cert path (mTLS) |
| `AURORA_TLS_KEY_PATH` | empty | client key path (mTLS) |

## Logging

| Variable | Default |
|---|---|
| `AURORA_LOG_JSON` | `true` |
| `AURORA_LOG_LEVEL` | `info` (`debug`, `warn`, `error`) |

## Example env file

```env
AURORA_NODE_ID=node-a1
AURORA_LIBVIRT_URI=qemu+unix:///system

AURORA_STREAM_MODE=grpc
AURORA_BACKEND_GRPC_ADDR=10.10.10.20:8443
AURORA_BACKEND_TOKEN=replace_me

AURORA_TLS_ENABLED=true
AURORA_TLS_SKIP_VERIFY=false
AURORA_TLS_CA_PATH=/etc/aurora/ca.pem
AURORA_TLS_CERT_PATH=/etc/aurora/client.crt
AURORA_TLS_KEY_PATH=/etc/aurora/client.key

AURORA_VM_POLL_INTERVAL=1s
AURORA_NODE_POLL_INTERVAL=3s
AURORA_HEALTH_INTERVAL=10s

AURORA_LOG_JSON=true
AURORA_LOG_LEVEL=info
```
