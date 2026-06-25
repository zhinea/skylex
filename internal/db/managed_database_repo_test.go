package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zhinea/skylex/internal/id"
)

func TestMigrations_SQLite_PostgresDatabasesTable(t *testing.T) {
	database, _ := newTestDB(t)
	ctx := context.Background()

	_, err := database.Conn().ExecContext(ctx,
		`SELECT id, cluster_id, database_name, owner_role_id, status, created_at, updated_at
		 FROM managed_databases LIMIT 0`)
	if err != nil {
		t.Fatalf("managed_databases table missing expected columns: %v", err)
	}
}

func TestPostgresDatabaseRepository_CreateListAndUniqueName(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewPostgresDatabaseRepository(database.Conn(), log)
	ctx := context.Background()
	clusterID := insertTestCluster(t, database, "managed-db-create")

	payload, _ := json.Marshal(map[string]string{
		"database_id":   "db-1",
		"operation_id":  "op-1",
		"database_name": "app_db",
	})
	created, err := repo.CreateWithCommand(ctx, CreateDatabaseTxInput{
		DatabaseID:   "db-1",
		OperationID:  "op-1",
		CommandID:    "cmd-1",
		ClusterID:    clusterID,
		AgentID:      "agent-1",
		DatabaseName: "app_db",
		Payload:      string(payload),
		EnsureAction: "pg_ensure_database",
	})
	if err != nil {
		t.Fatalf("create database with command: %v", err)
	}
	if created.Database.Status != "pending" || created.Operation.Status != "running" || created.Command.Action != "pg_ensure_database" {
		t.Fatalf("unexpected tx result: %#v", created)
	}

	items, err := repo.ListByCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	if len(items) != 1 || items[0].DatabaseName != "app_db" {
		t.Fatalf("unexpected databases: %#v", items)
	}

	_, err = repo.CreateWithCommand(ctx, CreateDatabaseTxInput{
		DatabaseID:   "db-2",
		OperationID:  "op-2",
		CommandID:    "cmd-2",
		ClusterID:    clusterID,
		AgentID:      "agent-1",
		DatabaseName: "app_db",
		Payload:      string(payload),
	})
	if err == nil {
		t.Fatal("expected duplicate database name to fail")
	}
}

func TestPostgresDatabaseRepository_HandleEnsureAndGrantResults(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewPostgresDatabaseRepository(database.Conn(), log)
	ctx := context.Background()
	clusterID := insertTestCluster(t, database, "managed-db-results")

	roleID := id.New()
	if _, err := database.Conn().ExecContext(ctx,
		Rebind(`INSERT INTO managed_roles
		 (id, cluster_id, role_name, role_kind, encrypted_password, password_version, status, created_at, updated_at)
		 VALUES (?, ?, 'app_user', 'read_write', 'ciphertext', 1, 'ready', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`),
		roleID, clusterID,
	); err != nil {
		t.Fatalf("insert owner role: %v", err)
	}

	ensurePayload, _ := json.Marshal(map[string]string{
		"database_id":     "db-grant",
		"operation_id":    "op-grant",
		"database_name":   "app_db",
		"owner_role_name": "app_user",
		"owner_role_kind": "read_write",
	})
	if _, err := repo.CreateWithCommand(ctx, CreateDatabaseTxInput{
		DatabaseID:   "db-grant",
		OperationID:  "op-grant",
		CommandID:    "cmd-ensure",
		ClusterID:    clusterID,
		AgentID:      "agent-1",
		DatabaseName: "app_db",
		OwnerRoleID:  roleID,
		Payload:      string(ensurePayload),
		EnsureAction: "pg_ensure_database",
	}); err != nil {
		t.Fatalf("create database: %v", err)
	}

	handled, grant, err := repo.HandleCommandResult(ctx, "cmd-ensure", true, "")
	if err != nil {
		t.Fatalf("handle ensure result: %v", err)
	}
	if !handled || grant == nil || grant.GrantRoleName != "app_user" || grant.ClusterID != clusterID {
		t.Fatalf("unexpected grant follow-up: handled=%v grant=%#v", handled, grant)
	}

	grantPayload, _ := json.Marshal(map[string]string{
		"database_id":     grant.DatabaseID,
		"operation_id":    grant.OperationID,
		"database_name":   grant.DatabaseName,
		"grant_role_name": grant.GrantRoleName,
		"grant_role_kind": grant.GrantRoleKind,
	})
	cmd, err := repo.QueueGrantCommand(ctx, GrantDatabaseTxInput{CommandID: "cmd-grant", AgentID: "agent-1", Payload: string(grantPayload), GrantAction: "pg_grant_database_privileges"})
	if err != nil {
		t.Fatalf("queue grant command: %v", err)
	}
	if cmd.Action != "pg_grant_database_privileges" {
		t.Fatalf("unexpected grant action: %q", cmd.Action)
	}
	if handled, _, err = repo.HandleCommandResult(ctx, "cmd-grant", true, ""); err != nil || !handled {
		t.Fatalf("handle grant result: handled=%v err=%v", handled, err)
	}

	managedDB, err := repo.GetByID(ctx, "db-grant")
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	if managedDB.Status != "ready" {
		t.Fatalf("expected ready database, got %q", managedDB.Status)
	}
	var opStatus string
	if err := database.Conn().QueryRowContext(ctx, Rebind(`SELECT status FROM service_operations WHERE id = ?`), "op-grant").Scan(&opStatus); err != nil {
		t.Fatalf("get operation status: %v", err)
	}
	if opStatus != "succeeded" {
		t.Fatalf("expected succeeded operation, got %q", opStatus)
	}
}

func TestPostgresDatabaseRepository_DeleteWithCommandAndDropResult(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewPostgresDatabaseRepository(database.Conn(), log)
	ctx := context.Background()
	clusterID := insertTestCluster(t, database, "managed-db-delete")

	createPayload, _ := json.Marshal(map[string]string{
		"database_id":   "db-delete",
		"operation_id":  "op-create",
		"database_name": "delete_me",
	})
	if _, err := repo.CreateWithCommand(ctx, CreateDatabaseTxInput{
		DatabaseID:   "db-delete",
		OperationID:  "op-create",
		CommandID:    "cmd-create",
		ClusterID:    clusterID,
		AgentID:      "agent-1",
		DatabaseName: "delete_me",
		Payload:      string(createPayload),
		EnsureAction: "pg_ensure_database",
	}); err != nil {
		t.Fatalf("create database: %v", err)
	}

	dropPayload, _ := json.Marshal(map[string]string{
		"database_id":   "db-delete",
		"operation_id":  "op-delete",
		"database_name": "delete_me",
	})
	if _, err := repo.DeleteWithCommand(ctx, DeleteDatabaseTxInput{
		DatabaseID:  "db-delete",
		OperationID: "op-delete",
		CommandID:   "cmd-drop",
		AgentID:     "agent-1",
		Payload:     string(dropPayload),
		DropAction:  "pg_drop_database",
	}); err != nil {
		t.Fatalf("delete database with command: %v", err)
	}

	managedDB, err := repo.GetByID(ctx, "db-delete")
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	if managedDB.Status != "deleting" {
		t.Fatalf("expected deleting status, got %q", managedDB.Status)
	}
	if handled, _, err := repo.HandleCommandResult(ctx, "cmd-drop", true, ""); err != nil || !handled {
		t.Fatalf("handle drop result: handled=%v err=%v", handled, err)
	}
	managedDB, err = repo.GetByID(ctx, "db-delete")
	if err != nil {
		t.Fatalf("get deleted database: %v", err)
	}
	if managedDB != nil {
		t.Fatalf("expected database row to be deleted, got %#v", managedDB)
	}
}
