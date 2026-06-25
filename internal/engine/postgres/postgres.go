// Package postgres provides the PostgreSQL implementation of engine.Provider.
// It is the first concrete provider; MariaDB/MySQL will follow the same shape.
package postgres

import (
	"fmt"
	"regexp"

	"github.com/zhinea/skylex/internal/engine"
	"github.com/zhinea/skylex/internal/models"
)

// roleNamePattern allows only safe PostgreSQL identifier characters.
var roleNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]{0,62}$`)

// databaseNamePattern allows safe PostgreSQL identifier characters for database names.
var databaseNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// modules is the ordered UI capability set for PostgreSQL. It advertises
// "extensions", which is PostgreSQL-specific and will be absent from the
// MariaDB provider.
var modules = []engine.Module{
	{ID: engine.ModuleOverview, Label: "Overview"},
	{ID: engine.ModuleConnection, Label: "Connection"},
	{ID: engine.ModuleDatabases, Label: "Databases"},
	{ID: engine.ModuleRoles, Label: "Roles & Users"},
	{ID: engine.ModuleNetwork, Label: "Network Security"},
	{ID: engine.ModuleTLS, Label: "TLS Encryption"},
	{ID: engine.ModuleExtensions, Label: "Extensions"},
	{ID: engine.ModuleSettings, Label: "Settings"},
	{ID: engine.ModuleDiagnostics, Label: "Diagnostics & Logs"},
}

// actions maps logical operations to PostgreSQL agent command action strings.
var actions = map[engine.LogicalOp]string{
	engine.OpEnsureDatabase:          "pg_ensure_database",
	engine.OpDropDatabase:            "pg_drop_database",
	engine.OpGrantDatabasePrivileges: "pg_grant_database_privileges",
	engine.OpEnsureRole:              "pg_ensure_role",
	engine.OpRotateRolePassword:      "pg_rotate_role_password",
	engine.OpDropRole:                "pg_drop_role",
	engine.OpApplyHBA:                "pg_apply_hba",
	engine.OpApplyTLS:                "pg_apply_tls",
}

// Provider implements engine.Provider for PostgreSQL.
type Provider struct{}

func (Provider) Engine() models.EngineType { return models.EnginePostgreSQL }

func (Provider) Modules() []engine.Module { return modules }

func (Provider) Supports(id engine.ModuleID) bool {
	for _, m := range modules {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (Provider) Action(op engine.LogicalOp) (string, bool) {
	a, ok := actions[op]
	return a, ok
}

func (Provider) ValidateRoleName(name string) error {
	if !roleNamePattern.MatchString(name) {
		return fmt.Errorf("invalid role name: must match %s", roleNamePattern.String())
	}
	return nil
}

func (Provider) ValidateDatabaseName(name string) error {
	if !databaseNamePattern.MatchString(name) {
		return fmt.Errorf("invalid database name: must match %s", databaseNamePattern.String())
	}
	return nil
}

func (Provider) DefaultPort() int { return 5432 }

// init registers the PostgreSQL provider with the engine registry.
func init() {
	engine.Register(Provider{})
}
