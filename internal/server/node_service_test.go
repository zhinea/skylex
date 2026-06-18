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
