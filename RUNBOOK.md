# Runbook

## Service commands

```bash
sudo systemctl status aurora-kvm-agent
sudo systemctl restart aurora-kvm-agent
sudo journalctl -u aurora-kvm-agent -f
```

## Health checks

1. Agent process alive
2. Libvirt reachable (`qemu+unix:///system`)
3. Backend stream reachable
4. Metrics timestamps tăng liên tục

## Incident: libvirt disconnect loop

- Kiểm tra libvirt daemon:

```bash
sudo systemctl status libvirtd
sudo journalctl -u libvirtd -n 100 --no-pager
```

- Kiểm tra URI:
  - `AURORA_LIBVIRT_URI`

## Incident: backend stream fail

- Kiểm tra network + firewall
- Kiểm tra token hết hạn
- Kiểm tra TLS files tồn tại và đúng quyền đọc

## Upgrade

```bash
cd vm-agent
go build -o aurora-kvm-agent ./cmd/agent
sudo install -m 0755 aurora-kvm-agent /usr/local/bin/aurora-kvm-agent
sudo systemctl restart aurora-kvm-agent
```

## Rollback

- Khôi phục binary version trước đó
- Restart service
- Theo dõi `journalctl -u aurora-kvm-agent -f`
