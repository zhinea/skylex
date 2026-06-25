// Package engine provides the polymorphic abstraction over database service
// providers (PostgreSQL today, MariaDB/MySQL planned). It lets the control
// plane treat managed databases, roles, network access and TLS uniformly while
// each concrete engine supplies its own capability set, identifier validation
// rules and agent command action strings.
//
// The abstraction is intentionally small: it exists because there is a concrete
// second implementation (MariaDB) on the roadmap, not as speculative
// generality. Storage tables are engine-neutral; only the behaviour that
// genuinely differs per engine lives behind Provider.
package engine

import (
	"fmt"
	"sync"

	"github.com/zhinea/skylex/internal/models"
)

// ModuleID identifies a management capability surfaced in the UI. Universal
// modules (overview, connection, settings, diagnostics) are present for every
// engine; feature modules (databases, roles, network, tls, extensions) are
// advertised only by engines that implement them.
type ModuleID string

const (
	ModuleOverview    ModuleID = "overview"
	ModuleConnection  ModuleID = "connection"
	ModuleDatabases   ModuleID = "databases"
	ModuleRoles       ModuleID = "roles"
	ModuleNetwork     ModuleID = "network"
	ModuleTLS         ModuleID = "tls"
	ModuleExtensions  ModuleID = "extensions"
	ModuleSettings    ModuleID = "settings"
	ModuleDiagnostics ModuleID = "diagnostics"
)

// Module is a single UI-visible capability. The ordered slice returned by a
// Provider drives the cluster detail sidebar directly, so the UI needs no
// engine-specific branching.
type Module struct {
	ID    ModuleID
	Label string
}

// LogicalOp is an engine-neutral management operation. Providers translate it
// into the concrete agent command action string they understand (e.g.
// OpEnsureDatabase -> "pg_ensure_database" for PostgreSQL).
type LogicalOp string

const (
	OpEnsureDatabase          LogicalOp = "ensure_database"
	OpDropDatabase            LogicalOp = "drop_database"
	OpGrantDatabasePrivileges LogicalOp = "grant_database_privileges"
	OpEnsureRole              LogicalOp = "ensure_role"
	OpRotateRolePassword      LogicalOp = "rotate_role_password"
	OpDropRole                LogicalOp = "drop_role"
	OpApplyHBA                LogicalOp = "apply_hba"
	OpApplyTLS                LogicalOp = "apply_tls"
	OpApplyExtensions         LogicalOp = "apply_extensions"
)

// Extension describes a single togglable database extension surfaced in the
// Extensions module. The catalog of available extensions is engine-specific, so
// it is exposed via the optional ExtensionCatalog interface rather than the core
// Provider (a future MariaDB provider simply will not implement it).
type Extension struct {
	// Name is the canonical extension identifier passed to the engine (e.g.
	// "uuid-ossp", "pg_trgm").
	Name string
	// Label is a short human-readable name for the UI.
	Label string
	// Description explains what the extension does in one line.
	Description string
}

// ExtensionCatalog is an optional capability a Provider may implement when its
// engine supports the Extensions module. Keeping it separate from Provider means
// engines without extensions (MariaDB/MySQL) do not carry empty stubs, and the
// curated allowlist lives next to the engine that owns it.
type ExtensionCatalog interface {
	// AvailableExtensions returns the curated, ordered allowlist of extensions a
	// user may toggle. Only names in this list are accepted by the control plane.
	AvailableExtensions() []Extension
	// ValidateExtensionName reports whether name is an accepted extension. This
	// is the security boundary: the agent only ever runs CREATE/DROP EXTENSION
	// for names that pass here.
	ValidateExtensionName(name string) error
}

// Provider encapsulates everything that varies per database engine. A concrete
// provider is stateless and safe for concurrent use.
type Provider interface {
	// Engine returns the engine this provider serves.
	Engine() models.EngineType

	// Modules returns the ordered set of UI modules this engine exposes.
	Modules() []Module

	// Supports reports whether the engine exposes the given module.
	Supports(ModuleID) bool

	// Action maps a logical operation to the agent command action string for
	// this engine. It returns ("", false) for operations the engine does not
	// support.
	Action(LogicalOp) (string, bool)

	// ValidateRoleName validates a managed role/user identifier.
	ValidateRoleName(string) error

	// ValidateDatabaseName validates a managed database identifier.
	ValidateDatabaseName(string) error

	// DefaultPort is the engine's default listening port.
	DefaultPort() int
}

// allOps is the exhaustive set of logical operations. It is used to build the
// reverse action lookup at registration time; add new operations here.
var allOps = []LogicalOp{
	OpEnsureDatabase,
	OpDropDatabase,
	OpGrantDatabasePrivileges,
	OpEnsureRole,
	OpRotateRolePassword,
	OpDropRole,
	OpApplyHBA,
	OpApplyTLS,
	OpApplyExtensions,
}

// actionRef identifies which engine and logical operation an agent command
// action string maps back to.
type actionRef struct {
	Engine models.EngineType
	Op     LogicalOp
}

// registry maps engine types to their providers. Concrete providers register
// themselves via Register in an init function. reverseActions maps each
// engine's concrete action strings back to a logical operation so command
// result handlers route engine-agnostically.
var (
	registryMu     sync.RWMutex
	registry       = map[models.EngineType]Provider{}
	reverseActions = map[string]actionRef{}
)

// Register adds a provider to the global registry. It panics on duplicate
// registration so wiring mistakes surface at startup rather than at runtime.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[p.Engine()]; exists {
		panic(fmt.Sprintf("engine: provider already registered for %q", p.Engine()))
	}
	registry[p.Engine()] = p
	for _, op := range allOps {
		action, ok := p.Action(op)
		if !ok {
			continue
		}
		if existing, dup := reverseActions[action]; dup {
			panic(fmt.Sprintf("engine: action %q already mapped to %q/%q", action, existing.Engine, existing.Op))
		}
		reverseActions[action] = actionRef{Engine: p.Engine(), Op: op}
	}
}

// LogicalOpForAction maps a concrete agent command action string back to its
// engine-neutral logical operation. Command result handlers use this to route
// without hardcoding per-engine action strings.
func LogicalOpForAction(action string) (LogicalOp, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ref, ok := reverseActions[action]
	return ref.Op, ok
}

// For returns the provider for the given engine, or an error if no provider is
// registered. Callers resolve the engine from clusters.engine.
func For(e models.EngineType) (Provider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[e]
	if !ok {
		return nil, fmt.Errorf("engine: no provider registered for %q", e)
	}
	return p, nil
}

// Engines returns the engine types that have a registered provider.
func Engines() []models.EngineType {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]models.EngineType, 0, len(registry))
	for e := range registry {
		out = append(out, e)
	}
	return out
}

// supports is a helper for providers to implement Supports from their module
// list without duplicating the lookup.
func supports(modules []Module, id ModuleID) bool {
	for _, m := range modules {
		if m.ID == id {
			return true
		}
	}
	return false
}
