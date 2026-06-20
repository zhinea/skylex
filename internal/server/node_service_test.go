package server

import (
	"context"
	"testing"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

func TestNodeService_DrainNodePersistsDrainedState(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)
	clusters := db.NewClusterRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	cluster, err := clusters.Create(context.Background(), "cluster-1", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 1, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node, err := nodes.Create(context.Background(), cluster.ID, "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	svc := NewNodeService(nodes, clusters, commands, db.NewCommandLogRepository(conn, log), 30*time.Second, log)
	resp, err := svc.DrainNode(context.Background(), &skylexv1.DrainNodeRequest{NodeId: node.ID})
	if err != nil {
		t.Fatalf("drain node: %v", err)
	}
	if resp.GetNode().GetStatus() != "drained" {
		t.Fatalf("expected drained response status, got %q", resp.GetNode().GetStatus())
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.Status != models.NodeStatusDrained {
		t.Fatalf("expected drained status, got %q", updated.Status)
	}
	if updated.StatusDetail != "drained" {
		t.Fatalf("expected drained detail, got %q", updated.StatusDetail)
	}

	pending, err := commands.ListPendingLimit(context.Background(), "agent-1", node.ID, 10)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	if len(pending) != 1 || pending[0].Action != "pg_stop" {
		t.Fatalf("expected one pg_stop command, got %#v", pending)
	}
}

func TestNodeService_DeleteNodeQueuesAgentDeactivation(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)
	clusters := db.NewClusterRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	svc := NewNodeService(nodes, clusters, commands, db.NewCommandLogRepository(conn, log), 30*time.Second, log)
	_, err = svc.DeleteNode(context.Background(), &skylexv1.DeleteNodeRequest{NodeId: node.ID})
	if err != nil {
		t.Fatalf("delete node: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated == nil {
		t.Fatal("expected node to remain until agent reports deactivation")
	}
	if updated.Status != models.NodeStatusDeleting {
		t.Fatalf("expected deleting status, got %q", updated.Status)
	}

	pending, err := commands.ListPendingLimit(context.Background(), "agent-1", node.ID, 10)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	if len(pending) != 1 || pending[0].Action != "agent_deactivate" {
		t.Fatalf("expected one agent_deactivate command, got %#v", pending)
	}
}

func TestNodeService_ListNodeMetricsReturnsRecentSamplesAscending(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	base := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 3; i++ {
		if err := nodes.InsertMetric(context.Background(), &models.NodeMetric{
			NodeID:          node.ID,
			RecordedAt:      base.Add(time.Duration(i) * time.Second),
			CPUUsagePercent: float64(i + 1),
		}); err != nil {
			t.Fatalf("insert metric: %v", err)
		}
	}

	svc := NewNodeService(nodes, db.NewClusterRepository(conn, log), db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), 30*time.Second, log)
	resp, err := svc.ListNodeMetrics(context.Background(), &skylexv1.ListNodeMetricsRequest{NodeId: node.ID, Limit: 2})
	if err != nil {
		t.Fatalf("list node metrics: %v", err)
	}
	metrics := resp.GetMetrics()
	if len(metrics) != 2 {
		t.Fatalf("expected two metrics, got %d", len(metrics))
	}
	if metrics[0].GetCpuUsagePercent() != 2 || metrics[1].GetCpuUsagePercent() != 3 {
		t.Fatalf("expected newest two samples in ascending order, got %#v", metrics)
	}
}

func TestAgentService_ReportAgentDeactivateDeletesNode(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	cmd, err := commands.Create(context.Background(), "agent-1", node.ID, "agent_deactivate", "")
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, commands, db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)
	_, err = svc.ReportCommandResult(context.Background(), &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-1",
		CommandId: cmd.ID,
		Success:   true,
		Output:    "deactivated",
	})
	if err != nil {
		t.Fatalf("report command result: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated != nil {
		t.Fatalf("expected node to be deleted, got %#v", updated)
	}
}
