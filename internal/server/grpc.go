package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type GRPCServer struct {
	server *grpc.Server
	addr   string
	log    *slog.Logger
}

func NewGRPCServer(srv *Server) (*GRPCServer, error) {
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcLoggingInterceptor(srv.log),
		),
	)

	reflection.Register(grpcServer)

	skylexv1.RegisterClusterServiceServer(grpcServer, srv.clusterService)
	skylexv1.RegisterNodeServiceServer(grpcServer, srv.nodeService)
	skylexv1.RegisterAgentServiceServer(grpcServer, srv.agentService)
	skylexv1.RegisterStorageServiceServer(grpcServer, srv.storageService)
	skylexv1.RegisterBackupServiceServer(grpcServer, srv.backupService)
	skylexv1.RegisterScheduleServiceServer(grpcServer, srv.backupService)

	addr := fmt.Sprintf("%s:%d", srv.cfg.Server.ListenAddr, srv.cfg.Server.GRPCPort)

	return &GRPCServer{
		server: grpcServer,
		addr:   addr,
		log:    srv.log,
	}, nil
}

func (g *GRPCServer) Serve(ctx context.Context) error {
	lis, err := net.Listen("tcp", g.addr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", g.addr, err)
	}

	g.log.Info("grpc server listening", "addr", g.addr)

	go func() {
		<-ctx.Done()
		g.log.Info("grpc server shutting down")
		g.server.GracefulStop()
	}()

	return g.server.Serve(lis)
}

func (g *GRPCServer) Shutdown(ctx context.Context) error {
	g.server.GracefulStop()
	return nil
}

func grpcLoggingInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		log.Debug("grpc request", "method", info.FullMethod)
		resp, err := handler(ctx, req)
		if err != nil {
			log.Error("grpc error", "method", info.FullMethod, "error", err)
		}
		return resp, err
	}
}