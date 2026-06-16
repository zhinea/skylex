package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/zhinea/skylex/internal/backup"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/dcs"
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
	authService      *AuthService
	backupWorker     *backup.Worker
	failoverEngine   *FailoverEngine
	dcsStore         *dcs.Store
	leaderElector    *dcs.LeaderElector
	workerCancel     context.CancelFunc
	failoverCancel   context.CancelFunc
	jwtManager       *JWTManager
	authInterceptor  *AuthInterceptor
	auditInterceptor *AuditInterceptor
	webhookClient    *WebhookClient
	metadataBackup   *MetadataBackup
	tlsConfig        *tls.Config
	auditRepo        *db.AuditRepository
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
		Driver:          s.cfg.Database.Driver,
		DSN:             s.cfg.Database.DSN,
		MaxOpenConns:    s.cfg.Database.MaxOpenConns,
		MaxIdleConns:    s.cfg.Database.MaxIdleConns,
		ConnMaxLifetime: s.cfg.Database.ConnMaxLifetime,
	}, s.log)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	s.db = database

	conn := database.Conn()

	clusterRepo := db.NewClusterRepository(conn, s.log)
	nodeRepo := db.NewNodeRepository(conn, s.log)
	commandRepo := db.NewAgentCommandRepository(conn, s.log)
	userRepo := db.NewUserRepository(conn, s.log)
	apiKeyRepo := db.NewAPIKeyRepository(conn, s.log)
	agentTokenRepo := db.NewAgentTokenRepository(conn, s.log)
	auditRepo := db.NewAuditRepository(conn, s.log)
	s.auditRepo = auditRepo

	s.jwtManager = NewJWTManager(s.cfg.Auth.JWTSecret, s.cfg.Auth.TokenExpiry, s.cfg.Auth.RefreshExpiry)
	s.authInterceptor = NewAuthInterceptor(s.jwtManager, apiKeyRepo, userRepo, s.log)
	s.auditInterceptor = NewAuditInterceptor(auditRepo, s.log)
	s.authService = NewAuthService(userRepo, apiKeyRepo, agentTokenRepo, s.jwtManager, s.log)

	s.webhookClient = NewWebhookClient(s.cfg.Webhook.URLs, s.cfg.Webhook.Timeout, s.log)

	s.clusterService = NewClusterService(clusterRepo, nodeRepo, commandRepo, s.log)
	s.nodeService = NewNodeService(nodeRepo, commandRepo, s.log)
	s.agentService = NewAgentService(nodeRepo, commandRepo, s.log)

	encryptKey := crypto.DeriveKey(s.cfg.Auth.JWTSecret, []byte("skylex-storage-key"))
	storageConfigRepo := db.NewStorageConfigRepository(conn, s.log, encryptKey)
	backupRepo := db.NewBackupRepository(conn, s.log)

	tlsConfig, err := LoadTLSCredentials(s.cfg.TLS)
	if err != nil {
		return fmt.Errorf("load tls credentials: %w", err)
	}
	s.tlsConfig = tlsConfig

	s.metadataBackup = NewMetadataBackup(database, s.cfg.Database.DSN, "backups/metadata", s.log)

	pgBackRest := backup.NewPgBackRest(s.cfg.Backup.PgBackRestPath, s.log)
	backupEngine := backup.NewEngine(backupRepo, storageConfigRepo, pgBackRest, s.log)
	backupWorker := backup.NewWorker(backupEngine, backupRepo, clusterRepo, s.log)

	s.storageService = NewStorageService(storageConfigRepo, s.log)
	s.backupService = NewBackupService(backupRepo, clusterRepo, backupEngine, backupWorker, s.log)
	s.backupWorker = backupWorker

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	workerCtx, workerCancel := context.WithCancel(ctx)
	s.workerCancel = workerCancel

	if err := backupWorker.Start(workerCtx); err != nil {
		return fmt.Errorf("start backup worker: %w", err)
	}

	dcsLog := s.log.With("component", "dcs")
	dcsStore, err := dcs.New(dcs.Config{
		Endpoints:   s.cfg.Etcd.Endpoints,
		DialTimeout: s.cfg.Etcd.DialTimeout,
		Username:    s.cfg.Etcd.Username,
		Password:    s.cfg.Etcd.Password,
	}, dcsLog)
	if err != nil {
		s.log.Warn("etcd connection failed, continuing without DCS", "error", err)
	} else {
		s.dcsStore = dcsStore

		candidateID, _ := os.Hostname()
		leaderElector := dcs.NewLeaderElector(dcsStore, dcsLog)

		leaderCtx, leaderCancel := context.WithCancel(context.Background())
		leaderCh, err := leaderElector.Campaign(leaderCtx, candidateID, 30)
		if err != nil {
			s.log.Warn("leader election campaign failed, continuing without leader", "error", err)
			leaderCancel()
		} else {
			s.leaderElector = leaderElector
			g.Go(func() error {
				defer leaderCancel()
				<-leaderCh
				s.log.Warn("lost leader election")
				return nil
			})
		}
	}

	s.failoverEngine = NewFailoverEngine(
		clusterRepo, nodeRepo, commandRepo,
		dcsStore, s.leaderElector,
		s.cfg.Agent.HeartbeatTimeout, s.log,
	)

	s.clusterService.SetFailoverEngine(s.failoverEngine)

	failoverCtx, failoverCancel := context.WithCancel(context.Background())
	s.failoverCancel = failoverCancel

	g.Go(func() error {
		s.failoverEngine.Run(failoverCtx)
		return nil
	})

	grpcServer, err := NewGRPCServer(s)
	if err != nil {
		return fmt.Errorf("init grpc server: %w", err)
	}
	s.grpc = grpcServer

	g.Go(func() error {
		return s.grpc.Serve(ctx)
	})

	g.Go(func() error {
		return s.serveConnectHTTP(ctx)
	})

	g.Go(func() error {
		return s.serveMetrics(ctx)
	})

	<-ctx.Done()
	s.log.Info("shutting down skylex server")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), s.cfg.Agent.HeartbeatTimeout)
	defer cancelShutdown()

	s.workerCancel()
	s.failoverCancel()

	if s.leaderElector != nil {
		s.leaderElector.Resign(shutdownCtx)
		s.leaderElector.Close()
	}

	if s.dcsStore != nil {
		s.dcsStore.Close()
	}

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