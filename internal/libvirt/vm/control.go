package vm

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	golibvirt "github.com/digitalocean/go-libvirt"

	"aurora-kvm-agent/internal/libvirt/metric"
)

const (
	defaultImageDir      = "/var/lib/libvirt/images"
	defaultNetworkName   = "default"
	defaultShutdownWaitS = 20
	defaultMigrateWaitS  = 600
)

type Controller struct {
	conn      *metric.ConnManager
	logger    *slog.Logger
	imageDir  string
	controlMu sync.Mutex
}

func NewController(conn *metric.ConnManager, logger *slog.Logger, imageDir string) *Controller {
	if strings.TrimSpace(imageDir) == "" {
		imageDir = defaultImageDir
	}
	return &Controller{
		conn:     conn,
		logger:   logger,
		imageDir: imageDir,
	}
}

func (c *Controller) CreateVM(ctx context.Context, spec CreateVMSpec) (VMOperationResult, error) {
	result := VMOperationResult{
		NodeID: spec.NodeID,
		VMName: strings.TrimSpace(spec.Name),
	}
	if err := validateCreateSpec(spec); err != nil {
		result.Message = err.Error()
		return result, nil
	}

	client, err := c.conn.Client(ctx)
	if err != nil {
		return result, err
	}

	c.controlMu.Lock()
	defer c.controlMu.Unlock()

	if _, err := client.DomainLookupByName(result.VMName); err == nil {
		result.Message = "vm already exists"
		return result, nil
	} else if !golibvirt.IsNotFound(err) {
		return result, fmt.Errorf("lookup vm %s: %w", result.VMName, err)
	}

	diskPath := strings.TrimSpace(spec.DiskPath)
	if diskPath == "" {
		diskPath = filepath.Join(c.imageDir, result.VMName+".qcow2")
	}

	admission, allow, rejectMsg, err := c.checkAdmission(ctx, client, admissionInput{
		excludeVMName: "",
		requestVCPU:   uint64(spec.VCPUCount),
		requestRAMMB:  spec.RAMMB,
		requestDiskB:  spec.DiskSizeGB * 1024 * 1024 * 1024,
		diskPath:      diskPath,
	})
	if err != nil {
		return result, err
	}
	result.Admission = admission
	if !allow {
		result.Rejected = true
		result.Message = rejectMsg
		return result, nil
	}

	if err := ensureDiskImage(ctx, diskPath, spec.DiskSizeGB); err != nil {
		return result, err
	}

	networkName := strings.TrimSpace(spec.NetworkName)
	if networkName == "" {
		networkName = defaultNetworkName
	}

	domainXML := buildDomainXML(result.VMName, spec.VCPUCount, spec.RAMMB, diskPath, networkName)
	dom, err := client.DomainDefineXML(domainXML)
	if err != nil {
		return result, fmt.Errorf("define vm %s: %w", result.VMName, err)
	}

	if spec.Autostart {
		if err := client.DomainSetAutostart(dom, 1); err != nil {
			return result, fmt.Errorf("set autostart vm %s: %w", result.VMName, err)
		}
	}

	if spec.StartNow {
		if err := client.DomainCreate(dom); err != nil {
			return result, fmt.Errorf("start vm %s: %w", result.VMName, err)
		}
	}

	result.OK = true
	result.Message = "vm created"
	return result, nil
}

func (c *Controller) UpdateVM(ctx context.Context, spec UpdateVMSpec) (VMOperationResult, error) {
	result := VMOperationResult{
		NodeID: spec.NodeID,
		VMName: strings.TrimSpace(spec.Name),
	}
	if strings.TrimSpace(spec.Name) == "" {
		result.Message = "vm name is required"
		return result, nil
	}

	client, err := c.conn.Client(ctx)
	if err != nil {
		return result, err
	}

	c.controlMu.Lock()
	defer c.controlMu.Unlock()

	dom, err := client.DomainLookupByName(result.VMName)
	if err != nil {
		if golibvirt.IsNotFound(err) {
			result.Message = "vm not found"
			return result, nil
		}
		return result, fmt.Errorf("lookup vm %s: %w", result.VMName, err)
	}

	vmCfg, err := c.readDomainConfig(client, dom)
	if err != nil {
		return result, fmt.Errorf("read vm config %s: %w", result.VMName, err)
	}

	desiredVCPU := vmCfg.VCPU
	if spec.VCPUCount > 0 {
		desiredVCPU = uint64(spec.VCPUCount)
	}

	desiredRAMMB := vmCfg.MemoryMB
	if spec.RAMMB > 0 {
		desiredRAMMB = spec.RAMMB
	}

	admission, allow, rejectMsg, err := c.checkAdmission(ctx, client, admissionInput{
		excludeVMName: result.VMName,
		requestVCPU:   desiredVCPU,
		requestRAMMB:  desiredRAMMB,
		requestDiskB:  0,
		diskPath:      c.imageDir,
	})
	if err != nil {
		return result, err
	}
	result.Admission = admission
	if !allow {
		result.Rejected = true
		result.Message = rejectMsg
		return result, nil
	}

	var changed bool
	if desiredVCPU != vmCfg.VCPU {
		flags := uint32(golibvirt.DomainVCPUConfig | golibvirt.DomainVCPULive)
		if err := client.DomainSetVcpusFlags(dom, uint32(desiredVCPU), flags); err != nil {
			// Fallback for shutoff domains.
			if cfgErr := client.DomainSetVcpusFlags(dom, uint32(desiredVCPU), uint32(golibvirt.DomainVCPUConfig)); cfgErr != nil {
				return result, fmt.Errorf("update vm vcpu %s: %w", result.VMName, err)
			}
		}
		changed = true
	}

	if desiredRAMMB != vmCfg.MemoryMB {
		memKiB := desiredRAMMB * 1024
		flags := uint32(golibvirt.DomainMemConfig | golibvirt.DomainMemLive)
		if err := client.DomainSetMemoryFlags(dom, memKiB, flags); err != nil {
			if cfgErr := client.DomainSetMemoryFlags(dom, memKiB, uint32(golibvirt.DomainMemConfig)); cfgErr != nil {
				return result, fmt.Errorf("update vm memory %s: %w", result.VMName, err)
			}
		}
		changed = true
	}

	if spec.Autostart != nil {
		autoVal := int32(0)
		if *spec.Autostart {
			autoVal = 1
		}
		if err := client.DomainSetAutostart(dom, autoVal); err != nil {
			return result, fmt.Errorf("update vm autostart %s: %w", result.VMName, err)
		}
		changed = true
	}

	result.OK = true
	if changed {
		result.Message = "vm updated"
	} else {
		result.Message = "no changes"
	}
	return result, nil
}

func (c *Controller) DeleteVM(ctx context.Context, spec DeleteVMSpec) (VMOperationResult, error) {
	result := VMOperationResult{
		NodeID: spec.NodeID,
		VMName: strings.TrimSpace(spec.Name),
	}
	if strings.TrimSpace(spec.Name) == "" {
		result.Message = "vm name is required"
		return result, nil
	}

	client, err := c.conn.Client(ctx)
	if err != nil {
		return result, err
	}

	c.controlMu.Lock()
	defer c.controlMu.Unlock()

	dom, err := client.DomainLookupByName(result.VMName)
	if err != nil {
		if golibvirt.IsNotFound(err) {
			result.OK = true
			result.Message = "vm not found"
			return result, nil
		}
		return result, fmt.Errorf("lookup vm %s: %w", result.VMName, err)
	}

	vmCfg, err := c.readDomainConfig(client, dom)
	if err != nil {
		return result, fmt.Errorf("read vm config %s: %w", result.VMName, err)
	}

	shutdownWait := spec.ShutdownTimeoutS
	if shutdownWait <= 0 {
		shutdownWait = defaultShutdownWaitS
	}

	state, _, _, _, _, infoErr := client.DomainGetInfo(dom)
	if infoErr == nil && isDomainRunning(state) {
		if err := client.DomainShutdown(dom); err != nil {
			if !spec.Force {
				return result, fmt.Errorf("shutdown vm %s: %w", result.VMName, err)
			}
		}
		stopped := waitDomainStopped(ctx, client, dom, time.Duration(shutdownWait)*time.Second)
		if !stopped {
			if !spec.Force {
				result.Message = "vm still running, retry with force=true"
				return result, nil
			}
			if err := client.DomainDestroy(dom); err != nil {
				return result, fmt.Errorf("destroy vm %s: %w", result.VMName, err)
			}
		}
	}

	undefFlags := golibvirt.DomainUndefineManagedSave |
		golibvirt.DomainUndefineSnapshotsMetadata |
		golibvirt.DomainUndefineCheckpointsMetadata
	if err := client.DomainUndefineFlags(dom, undefFlags); err != nil {
		if fallbackErr := client.DomainUndefine(dom); fallbackErr != nil {
			return result, fmt.Errorf("undefine vm %s: %w", result.VMName, err)
		}
	}

	if spec.RemoveDisk {
		for _, p := range vmCfg.DiskFiles {
			if strings.TrimSpace(p) == "" {
				continue
			}
			if rmErr := os.Remove(p); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				c.logger.Warn("remove vm disk failed", "vm", result.VMName, "disk", p, "error", rmErr)
			}
		}
	}

	result.OK = true
	result.Message = "vm deleted"
	return result, nil
}

func (c *Controller) MigrateVM(ctx context.Context, spec MigrateVMSpec) (VMOperationResult, error) {
	result := VMOperationResult{
		NodeID: spec.NodeID,
		VMName: strings.TrimSpace(spec.Name),
	}
	if result.VMName == "" {
		result.Message = "vm name is required"
		return result, nil
	}
	destURI := strings.TrimSpace(spec.DestinationURI)
	if destURI == "" {
		result.Message = "destination_uri is required"
		return result, nil
	}

	client, err := c.conn.Client(ctx)
	if err != nil {
		return result, err
	}

	c.controlMu.Lock()
	defer c.controlMu.Unlock()

	dom, err := client.DomainLookupByName(result.VMName)
	if err != nil {
		if golibvirt.IsNotFound(err) {
			result.Message = "vm not found"
			return result, nil
		}
		return result, fmt.Errorf("lookup vm %s: %w", result.VMName, err)
	}

	state, _, _, _, _, err := client.DomainGetInfo(dom)
	if err != nil {
		return result, fmt.Errorf("read vm state %s: %w", result.VMName, err)
	}
	running := isDomainRunning(state)

	if spec.Live && !running {
		c.logger.Info("live migrate requested but vm is not running, switching to offline migrate", "vm_name", result.VMName)
		spec.Live = false
	}

	timeoutS := spec.TimeoutSeconds
	if timeoutS <= 0 {
		timeoutS = defaultMigrateWaitS
	}
	migrateCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	defer cancel()

	output, migrateErr := runVirshMigrate(migrateCtx, spec)
	if migrateErr == nil {
		result.OK = true
		result.Message = "vm migrated"
		if output != "" {
			result.Message = result.Message + ": " + output
		}
		return result, nil
	}

	if spec.Live && spec.AllowOfflineFallback && running {
		c.logger.Warn(
			"live migrate failed, attempting offline fallback",
			"vm_name", result.VMName,
			"destination_uri", destURI,
			"error", migrateErr,
		)
		if err := client.DomainShutdown(dom); err == nil {
			_ = waitDomainStopped(ctx, client, dom, time.Duration(defaultShutdownWaitS)*time.Second)
		}

		fallback := spec
		fallback.Live = false
		fallback.Unsafe = true
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
		defer fallbackCancel()

		fallbackOut, fallbackErr := runVirshMigrate(fallbackCtx, fallback)
		if fallbackErr == nil {
			result.OK = true
			result.Message = "vm migrated with offline fallback"
			if fallbackOut != "" {
				result.Message = result.Message + ": " + fallbackOut
			}
			return result, nil
		}
		return result, fmt.Errorf("live migrate failed: %v; offline fallback failed: %w", migrateErr, fallbackErr)
	}

	return result, fmt.Errorf("migrate vm %s: %w", result.VMName, migrateErr)
}

type admissionInput struct {
	excludeVMName string
	requestVCPU   uint64
	requestRAMMB  uint64
	requestDiskB  uint64
	diskPath      string
}

func (c *Controller) checkAdmission(
	ctx context.Context,
	client *golibvirt.Libvirt,
	in admissionInput,
) (*AdmissionReport, bool, string, error) {
	totalVCPU, totalRAMMB, err := readNodeCapacity(client)
	if err != nil {
		return nil, false, "", err
	}

	usedVCPU, usedRAMMB, err := c.readAllocatedCapacity(client, in.excludeVMName)
	if err != nil {
		return nil, false, "", err
	}

	storagePath := in.diskPath
	if strings.TrimSpace(storagePath) == "" {
		storagePath = c.imageDir
	}
	storagePath = storageCheckPath(storagePath)
	storageTotal, storageFree, err := readStorageCapacity(storagePath)
	if err != nil {
		return nil, false, "", err
	}

	report := &AdmissionReport{
		TotalVCPU:      totalVCPU,
		UsedVCPU:       usedVCPU,
		RequestedVCPU:  in.requestVCPU,
		TotalRAMMB:     totalRAMMB,
		UsedRAMMB:      usedRAMMB,
		RequestedRAMMB: in.requestRAMMB,
		StoragePath:    storagePath,
		StorageTotalB:  storageTotal,
		StorageFreeB:   storageFree,
		RequestedDiskB: in.requestDiskB,
	}
	if totalVCPU > usedVCPU {
		report.RemainingVCPU = totalVCPU - usedVCPU
	}
	if totalRAMMB > usedRAMMB {
		report.RemainingRAMMB = totalRAMMB - usedRAMMB
	}

	if usedVCPU+in.requestVCPU > totalVCPU {
		return report, false, "insufficient cpu capacity", nil
	}
	if usedRAMMB+in.requestRAMMB > totalRAMMB {
		return report, false, "insufficient ram capacity", nil
	}
	if in.requestDiskB > 0 && storageFree < in.requestDiskB {
		return report, false, "insufficient storage capacity", nil
	}
	_ = ctx
	return report, true, "", nil
}

func readNodeCapacity(client *golibvirt.Libvirt) (totalVCPU uint64, totalRAMMB uint64, err error) {
	_, memKiB, cpus, _, _, _, _, _, err := client.NodeGetInfo()
	if err != nil {
		return 0, 0, fmt.Errorf("read node info: %w", err)
	}
	totalVCPU = uint64(cpus)
	totalRAMMB = memKiB / 1024
	return totalVCPU, totalRAMMB, nil
}

func readStorageCapacity(path string) (totalBytes uint64, freeBytes uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	bsize := uint64(st.Bsize)
	totalBytes = st.Blocks * bsize
	freeBytes = st.Bavail * bsize
	return totalBytes, freeBytes, nil
}

func storageCheckPath(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return defaultImageDir
	}
	info, err := os.Stat(clean)
	if err == nil && info.IsDir() {
		return clean
	}
	return filepath.Dir(clean)
}

func ensureDiskImage(ctx context.Context, path string, sizeGB uint64) error {
	if sizeGB == 0 {
		return errors.New("disk_size_gb must be > 0")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("disk image already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat disk image: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare disk directory: %w", err)
	}

	if _, err := exec.LookPath("qemu-img"); err != nil {
		return errors.New("qemu-img not found")
	}

	cmd := exec.CommandContext(ctx, "qemu-img", "create", "-f", "qcow2", path, fmt.Sprintf("%dG", sizeGB))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create disk image: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func waitDomainStopped(ctx context.Context, client *golibvirt.Libvirt, dom golibvirt.Domain, timeout time.Duration) bool {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		state, _, _, _, _, err := client.DomainGetInfo(dom)
		if err == nil && !isDomainRunning(state) {
			return true
		}
		select {
		case <-deadlineCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func isDomainRunning(state uint8) bool {
	switch golibvirt.DomainState(state) {
	case golibvirt.DomainRunning, golibvirt.DomainBlocked, golibvirt.DomainPaused, golibvirt.DomainPmsuspended:
		return true
	default:
		return false
	}
}

func validateCreateSpec(spec CreateVMSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("vm_name is required")
	}
	if spec.VCPUCount == 0 {
		return errors.New("vcpu_count must be > 0")
	}
	if spec.RAMMB == 0 {
		return errors.New("ram_mb must be > 0")
	}
	if spec.DiskSizeGB == 0 {
		return errors.New("disk_size_gb must be > 0")
	}
	return nil
}

func runVirshMigrate(ctx context.Context, spec MigrateVMSpec) (string, error) {
	if _, err := exec.LookPath("virsh"); err != nil {
		return "", errors.New("virsh not found")
	}

	vmName := strings.TrimSpace(spec.Name)
	args := []string{"migrate"}
	if spec.Live {
		args = append(args, "--live")
	}
	if spec.Persistent {
		args = append(args, "--persistent")
	}
	if spec.UndefineSource {
		args = append(args, "--undefinesource")
	}
	if spec.CopyStorageAll {
		args = append(args, "--copy-storage-all")
	}
	if spec.Unsafe {
		args = append(args, "--unsafe")
	}
	if spec.Compressed {
		args = append(args, "--compressed")
	}
	if spec.BandwidthMbps > 0 {
		args = append(args, "--bandwidth", strconv.FormatUint(spec.BandwidthMbps, 10))
	}
	args = append(args, "--verbose", vmName, strings.TrimSpace(spec.DestinationURI))

	if v := strings.TrimSpace(spec.MigrateURI); v != "" {
		args = append(args, v)
	}
	if v := strings.TrimSpace(spec.DestinationName); v != "" {
		args = append(args, v)
	}

	cmd := exec.CommandContext(ctx, "virsh", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output != "" {
			return output, fmt.Errorf("%w: %s", err, output)
		}
		return "", err
	}
	return output, nil
}

type domainDefinitionXML struct {
	XMLName       xml.Name          `xml:"domain"`
	Name          string            `xml:"name"`
	Memory        domainMemoryXML   `xml:"memory"`
	CurrentMemory domainMemoryXML   `xml:"currentMemory"`
	VCPU          domainVCPUXML     `xml:"vcpu"`
	OS            domainOSXML       `xml:"os"`
	Features      domainFeaturesXML `xml:"features"`
	CPU           domainCPUXML      `xml:"cpu"`
	Clock         domainClockXML    `xml:"clock"`
	OnPoweroff    string            `xml:"on_poweroff"`
	OnReboot      string            `xml:"on_reboot"`
	OnCrash       string            `xml:"on_crash"`
	Devices       domainDevicesXML  `xml:"devices"`
}

type domainMemoryXML struct {
	Unit  string `xml:"unit,attr,omitempty"`
	Value string `xml:",chardata"`
}

type domainVCPUXML struct {
	Placement string `xml:"placement,attr,omitempty"`
	Value     string `xml:",chardata"`
}

type domainOSXML struct {
	Type domainOSTypeXML `xml:"type"`
	Boot domainOSBootXML `xml:"boot"`
}

type domainOSTypeXML struct {
	Arch    string `xml:"arch,attr,omitempty"`
	Machine string `xml:"machine,attr,omitempty"`
	Type    string `xml:",chardata"`
}

type domainOSBootXML struct {
	Dev string `xml:"dev,attr"`
}

type domainFeaturesXML struct {
	ACPI struct{} `xml:"acpi"`
	APIC struct{} `xml:"apic"`
}

type domainCPUXML struct {
	Mode string `xml:"mode,attr,omitempty"`
}

type domainClockXML struct {
	Offset string `xml:"offset,attr,omitempty"`
}

type domainDevicesXML struct {
	Emulator   string                 `xml:"emulator"`
	Disks      []domainDeviceDiskXML  `xml:"disk"`
	Interfaces []domainDeviceIfaceXML `xml:"interface"`
	Console    domainDeviceConsoleXML `xml:"console"`
	Graphics   domainDeviceGraphicXML `xml:"graphics"`
}

type domainDeviceDiskXML struct {
	Type   string              `xml:"type,attr,omitempty"`
	Device string              `xml:"device,attr,omitempty"`
	Driver domainDiskDriverXML `xml:"driver"`
	Source domainDiskSourceXML `xml:"source"`
	Target domainDiskTargetXML `xml:"target"`
}

type domainDiskDriverXML struct {
	Name string `xml:"name,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type domainDiskSourceXML struct {
	File string `xml:"file,attr,omitempty"`
}

type domainDiskTargetXML struct {
	Dev string `xml:"dev,attr,omitempty"`
	Bus string `xml:"bus,attr,omitempty"`
}

type domainDeviceIfaceXML struct {
	Type   string               `xml:"type,attr,omitempty"`
	Source domainIfaceSourceXML `xml:"source"`
	Model  domainIfaceModelXML  `xml:"model"`
}

type domainIfaceSourceXML struct {
	Network string `xml:"network,attr,omitempty"`
}

type domainIfaceModelXML struct {
	Type string `xml:"type,attr,omitempty"`
}

type domainDeviceConsoleXML struct {
	Type string `xml:"type,attr,omitempty"`
}

type domainDeviceGraphicXML struct {
	Type     string `xml:"type,attr,omitempty"`
	Autoport string `xml:"autoport,attr,omitempty"`
	Listen   string `xml:"listen,attr,omitempty"`
}

func buildDomainXML(name string, vcpu uint32, ramMB uint64, diskPath string, networkName string) string {
	d := domainDefinitionXML{
		Name:          name,
		Memory:        domainMemoryXML{Unit: "MiB", Value: strconv.FormatUint(ramMB, 10)},
		CurrentMemory: domainMemoryXML{Unit: "MiB", Value: strconv.FormatUint(ramMB, 10)},
		VCPU:          domainVCPUXML{Placement: "static", Value: strconv.FormatUint(uint64(vcpu), 10)},
		OS: domainOSXML{
			Type: domainOSTypeXML{
				Arch:    "x86_64",
				Machine: "pc",
				Type:    "hvm",
			},
			Boot: domainOSBootXML{Dev: "hd"},
		},
		Features:   domainFeaturesXML{},
		CPU:        domainCPUXML{Mode: "host-passthrough"},
		Clock:      domainClockXML{Offset: "utc"},
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "restart",
		Devices: domainDevicesXML{
			Emulator: "/usr/bin/qemu-system-x86_64",
			Disks: []domainDeviceDiskXML{
				{
					Type:   "file",
					Device: "disk",
					Driver: domainDiskDriverXML{Name: "qemu", Type: "qcow2"},
					Source: domainDiskSourceXML{File: diskPath},
					Target: domainDiskTargetXML{Dev: "vda", Bus: "virtio"},
				},
			},
			Interfaces: []domainDeviceIfaceXML{
				{
					Type:   "network",
					Source: domainIfaceSourceXML{Network: networkName},
					Model:  domainIfaceModelXML{Type: "virtio"},
				},
			},
			Console:  domainDeviceConsoleXML{Type: "pty"},
			Graphics: domainDeviceGraphicXML{Type: "vnc", Autoport: "yes", Listen: "127.0.0.1"},
		},
	}
	out, _ := xml.Marshal(d)
	return string(out)
}

type parsedDomainConfig struct {
	Name      string
	VCPU      uint64
	MemoryMB  uint64
	DiskFiles []string
}

func (c *Controller) readAllocatedCapacity(client *golibvirt.Libvirt, excludeVMName string) (uint64, uint64, error) {
	doms, _, err := client.ConnectListAllDomains(1, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("list domains: %w", err)
	}
	var usedVCPU, usedRAMMB uint64
	for _, dom := range doms {
		cfg, err := c.readDomainConfig(client, dom)
		if err != nil {
			c.logger.Warn("read domain config failed, skipping", "domain", dom.Name, "error", err)
			continue
		}
		if excludeVMName != "" && cfg.Name == excludeVMName {
			continue
		}
		usedVCPU += cfg.VCPU
		usedRAMMB += cfg.MemoryMB
	}
	return usedVCPU, usedRAMMB, nil
}

func (c *Controller) readDomainConfig(client *golibvirt.Libvirt, dom golibvirt.Domain) (parsedDomainConfig, error) {
	xmlDesc, err := client.DomainGetXMLDesc(dom, 0)
	if err != nil {
		return parsedDomainConfig{}, err
	}
	var d domainConfigXML
	if err := xml.Unmarshal([]byte(xmlDesc), &d); err != nil {
		return parsedDomainConfig{}, fmt.Errorf("unmarshal domain xml: %w", err)
	}
	memMB, err := parseMemoryMB(d.Memory.Value, d.Memory.Unit)
	if err != nil {
		memMB = 0
	}
	if memMB == 0 {
		memMB, _ = parseMemoryMB(d.CurrentMemory.Value, d.CurrentMemory.Unit)
	}
	vcpu := parseUintDefault(d.VCPU.Value, 0)

	disks := make([]string, 0, len(d.Devices.Disks))
	for _, disk := range d.Devices.Disks {
		if strings.TrimSpace(disk.Source.File) != "" {
			disks = append(disks, strings.TrimSpace(disk.Source.File))
		}
	}
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = strings.TrimSpace(dom.Name)
	}
	return parsedDomainConfig{
		Name:      name,
		VCPU:      vcpu,
		MemoryMB:  memMB,
		DiskFiles: disks,
	}, nil
}

type domainConfigXML struct {
	Name          string                `xml:"name"`
	Memory        domainMemoryValueXML  `xml:"memory"`
	CurrentMemory domainMemoryValueXML  `xml:"currentMemory"`
	VCPU          domainVCPUValueXML    `xml:"vcpu"`
	Devices       domainConfigDeviceXML `xml:"devices"`
}

type domainMemoryValueXML struct {
	Unit  string `xml:"unit,attr"`
	Value string `xml:",chardata"`
}

type domainVCPUValueXML struct {
	Value string `xml:",chardata"`
}

type domainConfigDeviceXML struct {
	Disks []domainConfigDiskXML `xml:"disk"`
}

type domainConfigDiskXML struct {
	Source domainConfigDiskSourceXML `xml:"source"`
}

type domainConfigDiskSourceXML struct {
	File string `xml:"file,attr"`
}

func parseMemoryMB(value string, unit string) (uint64, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 0, nil
	}
	base, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "k", "kb", "kib":
		return base / 1024, nil
	case "m", "mb", "mib":
		return base, nil
	case "g", "gb", "gib":
		return base * 1024, nil
	case "b", "bytes":
		return base / (1024 * 1024), nil
	default:
		return base / 1024, nil
	}
}

func parseUintDefault(v string, fallback uint64) uint64 {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return fallback
	}
	u, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return u
}
