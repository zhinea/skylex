package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/zhinea/skylex/internal/backup"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	cfg              *Config
	log              *slog.Logger
	db               *db.DB
	http             *http.Server
	grpc             *GRPCServer
	clusterService   *ClusterService
	nodeService      *NodeService
	agentService     *AgentService
	storageService   *StorageService
	backupService    *BackupService
	backupWorker     *backup.Worker
	workerCancel     context.CancelFunc
}

func New(cfg *Config) (*Server, error) {
	log := newLogger(cfg.Logging.Level, cfg.Logging.Format)

	slog.SetDefault(log)

	s := &Server{
		cfg: cfg,
		log: log,
	}

	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.log.Info("starting skylex server",
		"version", "0.1.0",
		"grpc_port", s.cfg.Server.GRPCPort,
		"http_port", s.cfg.Server.HTTPPort,
		"metrics_port", s.cfg.Server.MetricsPort,
	)

	database, err := db.New(db.Config{
		Driver: s.cfg.Database.Driver,
		DSN:    s.cfg.Database.DSN,
	}, s.log)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	s.db = database

	conn := database.Conn()

	clusterRepo := db.NewClusterRepository(conn, s.log)
	nodeRepo := db.NewNodeRepository(conn, s.log)
	commandRepo := db.NewAgentCommandRepository(conn, s.log)

	s.clusterService = NewClusterService(clusterRepo, nodeRepo, commandRepo, s.log)
	s.nodeService = NewNodeService(nodeRepo, s.log)
	s.agentService = NewAgentService(nodeRepo, commandRepo, s.log)

	encryptKey := crypto.DeriveKey(s.cfg.Auth.JWTSecret, []byte("skylex-storage-key"))
	storageConfigRepo := db.NewStorageConfigRepository(conn, s.log, encryptKey)
	backupRepo := db.NewBackupRepository(conn, s.log)

	pgBackRest := backup.NewPgBackRest(s.cfg.Backup.PgBackRestPath, s.log)
	backupEngine := backup.NewEngine(backupRepo, storageConfigRepo, pgBackRest, s.log)
	backupWorker := backup.NewWorker(backupEngine, backupRepo, clusterRepo, s.log)

	s.storageService = NewStorageService(storageConfigRepo, s.log)
	s.backupService = NewBackupService(backupRepo, clusterRepo, backupEngine, backupWorker, s.log)
	s.backupWorker = backupWorker

	workerCtx, workerCancel := context.WithCancel(context.Background())
	s.workerCancel = workerCancel

	if err := backupWorker.Start(workerCtx); err != nil {
		return fmt.Errorf("start backup worker: %w", err)
	}

	grpcServer, err := NewGRPCServer(s)
	if err != nil {
		return fmt.Errorf("init grpc server: %w", err)
	}
	s.grpc = grpcServer

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.grpc.Serve(ctx)
	})

	g.Go(func() error {
		return s.serveMetrics(ctx)
	})

	<-ctx.Done()
	s.log.Info("shutting down skylex server")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), s.cfg.Agent.HeartbeatTimeout)
	defer cancelShutdown()

	s.workerCancel()

	if err := s.grpc.Shutdown(shutdownCtx); err != nil {
		s.log.Error("grpc shutdown error", "error", err)
	}

	if err := s.db.Close(); err != nil {
		s.log.Error("database close error", "error", err)
	}

	return g.Wait()
}

func (s *Server) DB() *db.DB {
	return s.db
}

func (s *Server) Config() *Config {
	return s.cfg
}

func (s *Server) Log() *slog.Logger {
	return s.log
}

func (s *Server) serveMetrics(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("# skylex metrics placeholder\n"))
	})

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.ListenAddr, s.cfg.Server.MetricsPort)
	httpServer := &http.Server{Addr: addr, Handler: mux}

	s.log.Info("metrics server listening", "addr", addr)

	go func() {
		<-ctx.Done()
		httpServer.Shutdown(context.Background())
	}()

	return httpServer.ListenAndServe()
}