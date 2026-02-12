package vm

import vmcontrol "aurora-kvm-agent/internal/libvirt/vm"

type CreateVMRequest struct {
	NodeID      string `json:"node_id"`
	VMName      string `json:"vm_name"`
	VCPUCount   uint32 `json:"vcpu_count"`
	RAMMB       uint64 `json:"ram_mb"`
	DiskSizeGB  uint64 `json:"disk_size_gb"`
	DiskPath    string `json:"disk_path"`
	NetworkName string `json:"network_name"`
	Autostart   bool   `json:"autostart"`
	StartNow    bool   `json:"start_now"`
}

type UpdateVMRequest struct {
	NodeID    string `json:"node_id"`
	VMName    string `json:"vm_name"`
	VCPUCount uint32 `json:"vcpu_count"`
	RAMMB     uint64 `json:"ram_mb"`
	Autostart *bool  `json:"autostart,omitempty"`
}

type DeleteVMRequest struct {
	NodeID           string `json:"node_id"`
	VMName           string `json:"vm_name"`
	Force            bool   `json:"force"`
	RemoveDisk       bool   `json:"remove_disk"`
	ShutdownTimeoutS int32  `json:"shutdown_timeout_s"`
}

type MigrateVMRequest struct {
	NodeID               string `json:"node_id"`
	VMName               string `json:"vm_name"`
	DestinationURI       string `json:"destination_uri"`
	MigrateURI           string `json:"migrate_uri"`
	DestinationName      string `json:"destination_name"`
	Live                 bool   `json:"live"`
	Persistent           bool   `json:"persistent"`
	UndefineSource       bool   `json:"undefine_source"`
	CopyStorageAll       bool   `json:"copy_storage_all"`
	Unsafe               bool   `json:"unsafe"`
	Compressed           bool   `json:"compressed"`
	AllowOfflineFallback bool   `json:"allow_offline_fallback"`
	BandwidthMbps        uint64 `json:"bandwidth_mbps"`
	TimeoutSeconds       int32  `json:"timeout_seconds"`
}

type VMOperationResponse struct {
	NodeID    string                     `json:"node_id"`
	VMName    string                     `json:"vm_name"`
	OK        bool                       `json:"ok"`
	Rejected  bool                       `json:"rejected"`
	Message   string                     `json:"message"`
	Admission *vmcontrol.AdmissionReport `json:"admission,omitempty"`
}
