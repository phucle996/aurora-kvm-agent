package vm

import (
	"context"
	"log/slog"

	vmcontrol "aurora-kvm-agent/internal/libvirt/vm"
)

func Create(
	ctx context.Context,
	logger *slog.Logger,
	controller *vmcontrol.Controller,
	req *CreateVMRequest,
) (*VMOperationResponse, error) {
	if req == nil {
		return &VMOperationResponse{OK: false, Message: "empty request"}, nil
	}
	result, err := controller.CreateVM(ctx, vmcontrol.CreateVMSpec{
		NodeID:      req.NodeID,
		Name:        req.VMName,
		VCPUCount:   req.VCPUCount,
		RAMMB:       req.RAMMB,
		DiskSizeGB:  req.DiskSizeGB,
		DiskPath:    req.DiskPath,
		NetworkName: req.NetworkName,
		Autostart:   req.Autostart,
		StartNow:    req.StartNow,
	})
	if err != nil {
		logger.Error("create vm rpc failed", "vm_name", req.VMName, "error", err)
		return nil, err
	}
	return &VMOperationResponse{
		NodeID:    result.NodeID,
		VMName:    result.VMName,
		OK:        result.OK,
		Rejected:  result.Rejected,
		Message:   result.Message,
		Admission: result.Admission,
	}, nil
}

func Update(
	ctx context.Context,
	logger *slog.Logger,
	controller *vmcontrol.Controller,
	req *UpdateVMRequest,
) (*VMOperationResponse, error) {
	if req == nil {
		return &VMOperationResponse{OK: false, Message: "empty request"}, nil
	}
	result, err := controller.UpdateVM(ctx, vmcontrol.UpdateVMSpec{
		NodeID:    req.NodeID,
		Name:      req.VMName,
		VCPUCount: req.VCPUCount,
		RAMMB:     req.RAMMB,
		Autostart: req.Autostart,
	})
	if err != nil {
		logger.Error("update vm rpc failed", "vm_name", req.VMName, "error", err)
		return nil, err
	}
	return &VMOperationResponse{
		NodeID:    result.NodeID,
		VMName:    result.VMName,
		OK:        result.OK,
		Rejected:  result.Rejected,
		Message:   result.Message,
		Admission: result.Admission,
	}, nil
}

func Delete(
	ctx context.Context,
	logger *slog.Logger,
	controller *vmcontrol.Controller,
	req *DeleteVMRequest,
) (*VMOperationResponse, error) {
	if req == nil {
		return &VMOperationResponse{OK: false, Message: "empty request"}, nil
	}
	result, err := controller.DeleteVM(ctx, vmcontrol.DeleteVMSpec{
		NodeID:           req.NodeID,
		Name:             req.VMName,
		Force:            req.Force,
		RemoveDisk:       req.RemoveDisk,
		ShutdownTimeoutS: req.ShutdownTimeoutS,
	})
	if err != nil {
		logger.Error("delete vm rpc failed", "vm_name", req.VMName, "error", err)
		return nil, err
	}
	return &VMOperationResponse{
		NodeID:    result.NodeID,
		VMName:    result.VMName,
		OK:        result.OK,
		Rejected:  result.Rejected,
		Message:   result.Message,
		Admission: result.Admission,
	}, nil
}

func Migrate(
	ctx context.Context,
	logger *slog.Logger,
	controller *vmcontrol.Controller,
	req *MigrateVMRequest,
) (*VMOperationResponse, error) {
	if req == nil {
		return &VMOperationResponse{OK: false, Message: "empty request"}, nil
	}
	result, err := controller.MigrateVM(ctx, vmcontrol.MigrateVMSpec{
		NodeID:               req.NodeID,
		Name:                 req.VMName,
		DestinationURI:       req.DestinationURI,
		MigrateURI:           req.MigrateURI,
		DestinationName:      req.DestinationName,
		Live:                 req.Live,
		Persistent:           req.Persistent,
		UndefineSource:       req.UndefineSource,
		CopyStorageAll:       req.CopyStorageAll,
		Unsafe:               req.Unsafe,
		Compressed:           req.Compressed,
		AllowOfflineFallback: req.AllowOfflineFallback,
		BandwidthMbps:        req.BandwidthMbps,
		TimeoutSeconds:       req.TimeoutSeconds,
	})
	if err != nil {
		logger.Error("migrate vm rpc failed", "vm_name", req.VMName, "destination_uri", req.DestinationURI, "error", err)
		return nil, err
	}
	return &VMOperationResponse{
		NodeID:    result.NodeID,
		VMName:    result.VMName,
		OK:        result.OK,
		Rejected:  result.Rejected,
		Message:   result.Message,
		Admission: result.Admission,
	}, nil
}
