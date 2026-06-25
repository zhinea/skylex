package agent

import (
	"context"
	"encoding/json"
	"fmt"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

// extensionsCommandPayload is the server-built payload for pg_apply_extensions.
// The server resolves the desired per-extension state and the set of target
// databases; the agent simply converges each database.
type extensionsCommandPayload struct {
	ClusterID string   `json:"cluster_id"`
	NodeID    string   `json:"node_id"`
	Databases []string `json:"databases"`
	// Enable / Disable are disjoint lists of extension names, already validated
	// against the engine allowlist by the control plane.
	Enable       []string `json:"enable"`
	Disable      []string `json:"disable"`
	AllowPromote bool     `json:"allow_promote"`
}

// executeApplyExtensions converges enabled/disabled extensions across the target
// databases. CREATE/DROP EXTENSION require no restart, so this is zero downtime.
// Work is a bounded sequential loop over a server-controlled list (databases x
// extensions) — no goroutine fan-out from request input.
func (a *Agent) executeApplyExtensions(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p extensionsCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_apply_extensions: invalid payload: %v", err)
	}
	if len(p.Databases) == 0 {
		// No managed databases yet: nothing to converge, but not an error — the
		// toggle state is recorded and will apply once databases exist.
		return true, "no target databases; extension state recorded", ""
	}

	applied := 0
	for _, dbName := range p.Databases {
		if dbName == "" {
			continue
		}
		for _, ext := range p.Enable {
			logger.Info(fmt.Sprintf("enabling extension %q in database %q", ext, dbName))
			if err := a.pg.EnsureExtension(ctx, dbName, ext, p.AllowPromote); err != nil {
				return false, "", fmt.Sprintf("pg_apply_extensions failed enabling %q in %q: %v", ext, dbName, err)
			}
			applied++
		}
		for _, ext := range p.Disable {
			logger.Info(fmt.Sprintf("disabling extension %q in database %q", ext, dbName))
			if err := a.pg.DropExtension(ctx, dbName, ext, p.AllowPromote); err != nil {
				return false, "", fmt.Sprintf("pg_apply_extensions failed disabling %q in %q: %v", ext, dbName, err)
			}
			applied++
		}
	}

	return true, fmt.Sprintf("extensions converged across %d database(s) (%d change(s))", len(p.Databases), applied), ""
}
