package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	agentversion "aurora-kvm-agent/internal/agent/version"
	agentvm "aurora-kvm-agent/internal/agent/vm"
	"aurora-kvm-agent/internal/config"
	vmcontrol "aurora-kvm-agent/internal/libvirt/vm"

	"google.golang.org/grpc"
	ggrpcencoding "google.golang.org/grpc/encoding"
)

func (a *Agent) runProbeListener(ctx context.Context) error {
	addr := a.cfg.ProbeListenAddr
	if addr == "" {
		return fmt.Errorf("empty probe listen address")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen probe endpoint %s: %w", addr, err)
	}
	defer func() { _ = ln.Close() }()

	a.logger.Info("probe endpoint listening", "addr", addr)

	registerAgentJSONCodec()
	server := grpc.NewServer()
	registerAgentServiceServer(server, &agentService{
		cfg:          a.cfg,
		logger:       a.logger,
		vmController: vmcontrol.NewController(a.conn, a.logger, ""),
	})

	go func() {
		<-ctx.Done()
		done := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			server.Stop()
		}
		_ = ln.Close()
	}()

	if err := server.Serve(ln); err != nil && ctx.Err() == nil {
		return fmt.Errorf("serve probe rpc endpoint %s: %w", addr, err)
	}
	return nil
}

type agentService struct {
	cfg          config.Config
	logger       *slog.Logger
	vmController *vmcontrol.Controller
}

func (s *agentService) GetVersion(ctx context.Context, req *agentversion.GetVersionRequest) (*agentversion.GetVersionResponse, error) {
	_ = ctx
	return agentversion.Get(s.cfg, req), nil
}

func (s *agentService) CreateVM(ctx context.Context, req *agentvm.CreateVMRequest) (*agentvm.VMOperationResponse, error) {
	return agentvm.Create(ctx, s.logger, s.vmController, req)
}

func (s *agentService) UpdateVM(ctx context.Context, req *agentvm.UpdateVMRequest) (*agentvm.VMOperationResponse, error) {
	return agentvm.Update(ctx, s.logger, s.vmController, req)
}

func (s *agentService) DeleteVM(ctx context.Context, req *agentvm.DeleteVMRequest) (*agentvm.VMOperationResponse, error) {
	return agentvm.Delete(ctx, s.logger, s.vmController, req)
}

func (s *agentService) MigrateVM(ctx context.Context, req *agentvm.MigrateVMRequest) (*agentvm.VMOperationResponse, error) {
	return agentvm.Migrate(ctx, s.logger, s.vmController, req)
}

type jsonCodec struct{}

func (jsonCodec) Name() string {
	return "json"
}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var registerAgentCodecOnce sync.Once

func registerAgentJSONCodec() {
	registerAgentCodecOnce.Do(func() {
		ggrpcencoding.RegisterCodec(jsonCodec{})
	})
}

type agentServiceServer interface {
	GetVersion(context.Context, *agentversion.GetVersionRequest) (*agentversion.GetVersionResponse, error)
	CreateVM(context.Context, *agentvm.CreateVMRequest) (*agentvm.VMOperationResponse, error)
	UpdateVM(context.Context, *agentvm.UpdateVMRequest) (*agentvm.VMOperationResponse, error)
	DeleteVM(context.Context, *agentvm.DeleteVMRequest) (*agentvm.VMOperationResponse, error)
	MigrateVM(context.Context, *agentvm.MigrateVMRequest) (*agentvm.VMOperationResponse, error)
}

func registerAgentServiceServer(s grpc.ServiceRegistrar, srv agentServiceServer) {
	s.RegisterService(&agentServiceDesc, srv)
}

var agentServiceDesc = grpc.ServiceDesc{
	ServiceName: "aurora.agent.v1.AgentService",
	HandlerType: (*agentServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetVersion",
			Handler:    _agentServiceGetVersionHandler,
		},
		{
			MethodName: "CreateVM",
			Handler:    _agentServiceCreateVMHandler,
		},
		{
			MethodName: "UpdateVM",
			Handler:    _agentServiceUpdateVMHandler,
		},
		{
			MethodName: "DeleteVM",
			Handler:    _agentServiceDeleteVMHandler,
		},
		{
			MethodName: "MigrateVM",
			Handler:    _agentServiceMigrateVMHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "metrics.proto",
}

func _agentServiceGetVersionHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(agentversion.GetVersionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(agentServiceServer).GetVersion(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/aurora.agent.v1.AgentService/GetVersion",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(agentServiceServer).GetVersion(ctx, req.(*agentversion.GetVersionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _agentServiceCreateVMHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(agentvm.CreateVMRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(agentServiceServer).CreateVM(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/aurora.agent.v1.AgentService/CreateVM",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(agentServiceServer).CreateVM(ctx, req.(*agentvm.CreateVMRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _agentServiceUpdateVMHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(agentvm.UpdateVMRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(agentServiceServer).UpdateVM(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/aurora.agent.v1.AgentService/UpdateVM",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(agentServiceServer).UpdateVM(ctx, req.(*agentvm.UpdateVMRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _agentServiceDeleteVMHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(agentvm.DeleteVMRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(agentServiceServer).DeleteVM(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/aurora.agent.v1.AgentService/DeleteVM",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(agentServiceServer).DeleteVM(ctx, req.(*agentvm.DeleteVMRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _agentServiceMigrateVMHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(agentvm.MigrateVMRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(agentServiceServer).MigrateVM(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/aurora.agent.v1.AgentService/MigrateVM",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(agentServiceServer).MigrateVM(ctx, req.(*agentvm.MigrateVMRequest))
	}
	return interceptor(ctx, in, info, handler)
}
