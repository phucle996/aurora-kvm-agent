package model

type AgentVersionRequest struct {
	NodeID string `json:"node_id"`
}

type AgentVersionResponse struct {
	NodeID          string `json:"node_id"`
	AgentVersion    string `json:"agent_version"`
	StreamMode      string `json:"stream_mode"`
	ProbeListenAddr string `json:"probe_listen_addr"`
	CheckedAtUnix   int64  `json:"checked_at_unix"`
}

type AgentCreateVMRequest struct {
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

type AgentUpdateVMRequest struct {
	NodeID    string `json:"node_id"`
	VMName    string `json:"vm_name"`
	VCPUCount uint32 `json:"vcpu_count"`
	RAMMB     uint64 `json:"ram_mb"`
	Autostart bool   `json:"autostart"`
}

type AgentDeleteVMRequest struct {
	NodeID           string `json:"node_id"`
	VMName           string `json:"vm_name"`
	Force            bool   `json:"force"`
	RemoveDisk       bool   `json:"remove_disk"`
	ShutdownTimeoutS int32  `json:"shutdown_timeout_s"`
}

type AgentMigrateVMRequest struct {
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

type AgentAdmissionReport struct {
	TotalVCPU          uint64 `json:"total_vcpu"`
	UsedVCPU           uint64 `json:"used_vcpu"`
	RemainingVCPU      uint64 `json:"remaining_vcpu"`
	RequestedVCPU      uint64 `json:"requested_vcpu"`
	TotalRAMMB         uint64 `json:"total_ram_mb"`
	UsedRAMMB          uint64 `json:"used_ram_mb"`
	RemainingRAMMB     uint64 `json:"remaining_ram_mb"`
	RequestedRAMMB     uint64 `json:"requested_ram_mb"`
	StoragePath        string `json:"storage_path"`
	StorageFreeBytes   uint64 `json:"storage_free_bytes"`
	StorageTotalBytes  uint64 `json:"storage_total_bytes"`
	RequestedDiskBytes uint64 `json:"requested_disk_bytes"`
}

type AgentVMOperationResponse struct {
	NodeID    string                `json:"node_id"`
	VMName    string                `json:"vm_name"`
	OK        bool                  `json:"ok"`
	Rejected  bool                  `json:"rejected"`
	Message   string                `json:"message"`
	Admission *AgentAdmissionReport `json:"admission,omitempty"`
}
