package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/dcs"
	"github.com/zhinea/skylex/internal/models"
)

type FailoverEngine struct {
	clusters         *db.ClusterRepository
	nodes            *db.NodeRepository
	commands         *db.AgentCommandRepository
	dcsStore         *dcs.Store
	elector          *dcs.LeaderElector
	heartbeatTimeout time.Duration
	log              *slog.Logger
}

func NewFailoverEngine(
	clusters *db.ClusterRepository,
	nodes *db.NodeRepository,
	commands *db.AgentCommandRepository,
	dcsStore *dcs.Store,
	elector *dcs.LeaderElector,
	heartbeatTimeout time.Duration,
	log *slog.Logger,
) *FailoverEngine {
	return &FailoverEngine{
		clusters:         clusters,
		nodes:            nodes,
		commands:         commands,
		dcsStore:         dcsStore,
		elector:          elector,
		heartbeatTimeout: heartbeatTimeout,
		log:              log,
	}
}

func (e *FailoverEngine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.heartbeatTimeout / 2)
	defer ticker.Stop()

	e.log.Info("failover engine started",
		"heartbeat_timeout", e.heartbeatTimeout,
	)

	for {
		select {
		case <-ctx.Done():
			e.log.Info("failover engine stopped")
			return
		case <-ticker.C:
			if !e.elector.IsLeader() {
				continue
			}
			e.monitorClusters(ctx)
		}
	}
}

func (e *FailoverEngine) monitorClusters(ctx context.Context) {
	clusters, _, err := e.clusters.List(ctx, 0, 100)
	if err != nil {
		e.log.Error("list clusters for health check", "error", err)
		return
	}

	for _, cluster := range clusters {
		if cluster.Status != models.ClusterStatusRunning &&
			cluster.Status != models.ClusterStatusDegraded {
			continue
		}
		e.checkClusterHealth(ctx, cluster)
	}
}

func (e *FailoverEngine) checkClusterHealth(ctx context.Context, cluster *models.Cluster) {
	primary, err := e.nodes.GetPrimary(ctx, cluster.ID)
	if err != nil || primary == nil {
		return
	}

	timeSinceHeartbeat := time.Since(primary.LastSeen)

	if timeSinceHeartbeat <= e.heartbeatTimeout {
		return
	}

	e.log.Warn("primary node appears offline, initiating failover",
		"cluster_id", cluster.ID,
		"cluster_name", cluster.Name,
		"primary_id", primary.ID,
		"primary_hostname", primary.Hostname,
		"time_since_heartbeat", timeSinceHeartbeat,
	)

	if primary.Status == models.NodeStatusPromoting {
		e.log.Info("primary is already being promoted, skipping failover",
			"cluster_id", cluster.ID,
			"primary_id", primary.ID,
		)
		return
	}

	e.executeFailover(ctx, cluster, primary)
}

func (e *FailoverEngine) executeFailover(ctx context.Context, cluster *models.Cluster, failedPrimary *models.Node) {
	e.log.Info("executing failover",
		"cluster_id", cluster.ID,
		"failed_primary", failedPrimary.Hostname,
	)

	if err := e.nodes.UpdateStatus(ctx, failedPrimary.ID, models.NodeStatusOffline); err != nil {
		e.log.Error("mark failed primary offline", "error", err)
		return
	}

	if err := e.clusters.UpdateStatus(ctx, cluster.ID, models.ClusterStatusDegraded); err != nil {
		e.log.Error("mark cluster degraded", "error", err)
		return
	}

	replicas, err := e.nodes.GetReplicas(ctx, cluster.ID)
	if err != nil {
		e.log.Error("get replicas for failover", "error", err)
		return
	}

	if len(replicas) == 0 {
		e.log.Error("no replicas available for failover", "cluster_id", cluster.ID)
		return
	}

	var bestReplica *models.Node
	for _, r := range replicas {
		if r.Status != models.NodeStatusOnline {
			continue
		}
		if bestReplica == nil || r.LastSeen.After(bestReplica.LastSeen) {
			bestReplica = r
		}
	}

	if bestReplica == nil {
		e.log.Error("no healthy replicas available for failover", "cluster_id", cluster.ID)
		return
	}

	e.log.Info("selected replica for promotion",
		"replica_id", bestReplica.ID,
		"replica_hostname", bestReplica.Hostname,
	)

	if err := e.nodes.UpdateStatus(ctx, bestReplica.ID, models.NodeStatusPromoting); err != nil {
		e.log.Error("mark replica promoting", "error", err)
		return
	}

	promoteCmdID, err := e.createCommand(ctx, bestReplica.ID, "pg_promote", "")
	if err != nil {
		e.log.Error("create promote command", "error", err)
		return
	}

	e.log.Info("sent promote command to replica",
		"command_id", promoteCmdID,
		"replica_id", bestReplica.ID,
	)

	time.Sleep(5 * time.Second)

	if err := e.nodes.UpdateRole(ctx, bestReplica.ID, models.NodeRolePrimary); err != nil {
		e.log.Error("update replica role to primary", "error", err)
	}

	if err := e.nodes.UpdateStatus(ctx, bestReplica.ID, models.NodeStatusOnline); err != nil {
		e.log.Error("update new primary status", "error", err)
	}

	if failedPrimary.ID != bestReplica.ID {
		if err := e.nodes.UpdateRole(ctx, failedPrimary.ID, models.NodeRoleReplica); err != nil {
			e.log.Error("update failed primary role to replica", "error", err)
		}
	}

	for _, replica := range replicas {
		if replica.ID == bestReplica.ID {
			continue
		}
		if replica.Status != models.NodeStatusOnline {
			continue
		}

		payload := fmt.Sprintf("%s:%d", bestReplica.Address, bestReplica.Port)
		e.createCommand(ctx, replica.ID, "repoint_replica", payload)
		e.log.Info("sent repoint command to replica",
			"replica_id", replica.ID,
			"new_primary", bestReplica.Address,
		)
	}

	if failedPrimary.ID != bestReplica.ID {
		payload := fmt.Sprintf("%s:%d", bestReplica.Address, bestReplica.Port)
		e.createCommand(ctx, failedPrimary.ID, "pg_rewind", payload)
		e.log.Info("sent rewind command to failed primary",
			"node_id", failedPrimary.ID,
			"new_primary", bestReplica.Address,
		)
	}

	if err := e.clusters.UpdateStatus(ctx, cluster.ID, models.ClusterStatusRunning); err != nil {
		e.log.Error("restore cluster status", "error", err)
	}

	if e.dcsStore != nil {
		primaryInfo := dcs.PrimaryInfo{
			ClusterID: cluster.ID,
			NodeID:    bestReplica.ID,
			Hostname:  bestReplica.Hostname,
			Address:   bestReplica.Address,
			Port:      bestReplica.Port,
		}
		if err := e.dcsStore.SetPrimary(ctx, cluster.ID, primaryInfo); err != nil {
			e.log.Error("update etcd primary info", "error", err)
		}
	}

	e.log.Info("failover completed successfully",
		"cluster_id", cluster.ID,
		"new_primary", bestReplica.Hostname,
	)
}

func (e *FailoverEngine) createCommand(ctx context.Context, nodeID, action, payload string) (string, error) {
	node, err := e.nodes.GetByID(ctx, nodeID)
	if err != nil || node == nil {
		return "", fmt.Errorf("node not found: %s", nodeID)
	}

	agentID := node.AgentID
	if agentID == "" {
		agentID = "failover-sentinel"
	}

	cmd, err := e.commands.Create(ctx, agentID, nodeID, action, payload)
	if err != nil {
		return "", fmt.Errorf("create command: %w", err)
	}

	return cmd.ID, nil
}