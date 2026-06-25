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
	engine.OpApplyExtensions:         "pg_apply_extensions",
}

// extensionNamePattern allows safe PostgreSQL extension identifier characters.
// Extension names are double-quoted before use, but we still constrain the
// charset so only known-shaped identifiers reach the engine.
var extensionNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,62}$`)

// availableExtensions is the curated allowlist of extensions a user may toggle.
// All of these ship with the standard postgresql-contrib package and are enabled
// with a plain CREATE EXTENSION — they do NOT require shared_preload_libraries or
// a server restart, which keeps "apply" a zero-downtime operation. Extensions
// that need a restart (e.g. pg_stat_statements) are intentionally excluded from
// this first cut rather than silently triggering a restart.
var availableExtensions = []engine.Extension{
	{Name: "uuid-ossp", Label: "uuid-ossp", Description: "Generate universally unique identifiers (UUIDs)."},
	{Name: "pgcrypto", Label: "pgcrypto", Description: "Cryptographic functions (hashing, encryption, random)."},
	{Name: "pg_trgm", Label: "pg_trgm", Description: "Trigram-based fuzzy text search and similarity."},
	{Name: "citext", Label: "citext", Description: "Case-insensitive text data type."},
	{Name: "hstore", Label: "hstore", Description: "Key/value pair storage within a single column."},
	{Name: "btree_gin", Label: "btree_gin", Description: "GIN index support for common scalar types."},
	{Name: "btree_gist", Label: "btree_gist", Description: "GiST index support for common scalar types."},
	{Name: "unaccent", Label: "unaccent", Description: "Text search dictionary that removes accents."},
	{Name: "ltree", Label: "ltree", Description: "Data type for hierarchical tree-like structures."},
	{Name: "tablefunc", Label: "tablefunc", Description: "Cross-tab (pivot) and other table functions."},
}

// extensionAllowSet indexes availableExtensions for O(1) validation.
var extensionAllowSet = func() map[string]bool {
	m := make(map[string]bool, len(availableExtensions))
	for _, e := range availableExtensions {
		m[e.Name] = true
	}
	return m
}()

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

// AvailableExtensions implements engine.ExtensionCatalog.
func (Provider) AvailableExtensions() []engine.Extension { return availableExtensions }

// ValidateExtensionName implements engine.ExtensionCatalog. It only accepts
// names from the curated allowlist, and additionally enforces the identifier
// charset as defense in depth.
func (Provider) ValidateExtensionName(name string) error {
	if !extensionAllowSet[name] {
		return fmt.Errorf("extension %q is not in the supported allowlist", name)
	}
	if !extensionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid extension name: must match %s", extensionNamePattern.String())
	}
	return nil
}

// init registers the PostgreSQL provider with the engine registry.
func init() {
	engine.Register(Provider{})
}
