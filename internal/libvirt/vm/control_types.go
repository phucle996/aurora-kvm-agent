package vm

type CreateVMSpec struct {
	NodeID      string
	Name        string
	VCPUCount   uint32
	RAMMB       uint64
	DiskSizeGB  uint64
	DiskPath    string
	NetworkName string
	Autostart   bool
	StartNow    bool
}

type UpdateVMSpec struct {
	NodeID    string
	Name      string
	VCPUCount uint32
	RAMMB     uint64
	Autostart *bool
}

type DeleteVMSpec struct {
	NodeID           string
	Name             string
	Force            bool
	RemoveDisk       bool
	ShutdownTimeoutS int32
}

type MigrateVMSpec struct {
	NodeID               string
	Name                 string
	DestinationURI       string
	MigrateURI           string
	DestinationName      string
	Live                 bool
	Persistent           bool
	UndefineSource       bool
	CopyStorageAll       bool
	Unsafe               bool
	Compressed           bool
	AllowOfflineFallback bool
	BandwidthMbps        uint64
	TimeoutSeconds       int32
}

type AdmissionReport struct {
	TotalVCPU      uint64 `json:"total_vcpu"`
	UsedVCPU       uint64 `json:"used_vcpu"`
	RemainingVCPU  uint64 `json:"remaining_vcpu"`
	RequestedVCPU  uint64 `json:"requested_vcpu"`
	TotalRAMMB     uint64 `json:"total_ram_mb"`
	UsedRAMMB      uint64 `json:"used_ram_mb"`
	RemainingRAMMB uint64 `json:"remaining_ram_mb"`
	RequestedRAMMB uint64 `json:"requested_ram_mb"`
	StoragePath    string `json:"storage_path"`
	StorageFreeB   uint64 `json:"storage_free_bytes"`
	StorageTotalB  uint64 `json:"storage_total_bytes"`
	RequestedDiskB uint64 `json:"requested_disk_bytes"`
}

type VMOperationResult struct {
	NodeID    string           `json:"node_id"`
	VMName    string           `json:"vm_name"`
	OK        bool             `json:"ok"`
	Rejected  bool             `json:"rejected"`
	Message   string           `json:"message"`
	Admission *AdmissionReport `json:"admission,omitempty"`
}
