package postgres

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const skylexHBAFileName = "skylex.pg_hba.conf"

const (
	skylexHBABegin = "# BEGIN SKYLEX MANAGED HBA"
	skylexHBAEnd   = "# END SKYLEX MANAGED HBA"
)

// HBAPolicy describes the explicit network allowlists Skylex manages.
type HBAPolicy struct {
	AdminCIDRs       []string
	ReplicationCIDRs []string
	ApplicationCIDRs []string
	AdminRoles       []string
	ApplicationRoles []string
	ApplicationDBs   []string
}

// ApplyHBA writes a deterministic Skylex-managed HBA include file, reloads
// PostgreSQL, and restores the previous file if reload fails.
func (p *Instance) ApplyHBA(ctx context.Context, policy HBAPolicy) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if !p.IsInitialized() {
		return fmt.Errorf("data directory is not initialized")
	}

	includePath := filepath.Join(p.DataDir, skylexHBAFileName)
	previousInclude, readErr := os.ReadFile(includePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read previous hba include: %w", readErr)
	}
	previousIncludeExists := readErr == nil

	mainPath := filepath.Join(p.DataDir, "pg_hba.conf")
	previousMain, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("read pg_hba.conf: %w", err)
	}

	next := []byte(RenderHBAPolicy(policy, p.Superuser, p.ReplUser))
	nextMain := []byte(withManagedHBAPrefix(string(previousMain), p.Superuser))
	if previousIncludeExists && bytes.Equal(previousInclude, next) && bytes.Equal(previousMain, nextMain) {
		p.log.Info("skylex hba include already up to date", "path", includePath)
		return nil
	}

	if err := os.WriteFile(includePath, next, 0600); err != nil {
		return fmt.Errorf("write hba include: %w", err)
	}
	if err := os.WriteFile(mainPath, nextMain, 0600); err != nil {
		restoreHBAInclude(includePath, previousInclude, previousIncludeExists)
		return fmt.Errorf("write pg_hba.conf managed prefix: %w", err)
	}
	if err := p.Reload(ctx); err != nil {
		restoreHBAInclude(includePath, previousInclude, previousIncludeExists)
		_ = os.WriteFile(mainPath, previousMain, 0600)
		return fmt.Errorf("reload after hba apply failed; previous HBA configuration restored: %w", err)
	}
	return nil
}

// RenderHBAPolicy returns deterministic pg_hba.conf lines for testing and agent application.
func RenderHBAPolicy(policy HBAPolicy, superuser, replUser string) string {
	admin := canonicalStrings(policy.AdminCIDRs)
	replication := canonicalStrings(policy.ReplicationCIDRs)
	application := canonicalStrings(policy.ApplicationCIDRs)
	adminRoles := append([]string{hbaField(superuser)}, canonicalStrings(policy.AdminRoles)...)
	applicationRoles := hbaList(policy.ApplicationRoles)
	applicationDBs := hbaList(policy.ApplicationDBs)

	var b strings.Builder
	b.WriteString("# Managed by Skylex. Do not edit manually.\n")
	b.WriteString("# Local administrative access for the PostgreSQL service account.\n")
	b.WriteString("local   all             all                                     scram-sha-256\n")
	b.WriteString("host    all             ")
	b.WriteString(hbaField(superuser))
	b.WriteString("             127.0.0.1/32            scram-sha-256\n")
	b.WriteString("host    all             ")
	b.WriteString(hbaField(superuser))
	b.WriteString("             ::1/128                 scram-sha-256\n")

	for _, cidr := range admin {
		for _, role := range adminRoles {
			b.WriteString(fmt.Sprintf("host    all             %s             %-22s scram-sha-256\n", role, cidr))
		}
	}
	for _, cidr := range replication {
		b.WriteString(fmt.Sprintf("host    replication     %s             %-22s scram-sha-256\n", hbaField(replUser), cidr))
	}
	if applicationRoles != "" && applicationDBs != "" {
		for _, cidr := range application {
			b.WriteString(fmt.Sprintf("host    %s             %s             %-22s scram-sha-256\n", applicationDBs, applicationRoles, cidr))
		}
	}
	return b.String()
}

func withManagedHBAPrefix(content, superuser string) string {
	directive := fmt.Sprintf("include_if_exists %s", skylexHBAFileName)
	managed := strings.Join([]string{
		skylexHBABegin,
		fmt.Sprintf("local   all             %-39s scram-sha-256", hbaField(superuser)),
		fmt.Sprintf("host    all             %-16s 127.0.0.1/32            scram-sha-256", hbaField(superuser)),
		fmt.Sprintf("host    all             %-16s ::1/128                 scram-sha-256", hbaField(superuser)),
		directive,
		"host    all             all             0.0.0.0/0               reject",
		"host    all             all             ::/0                    reject",
		"host    replication     all             0.0.0.0/0               reject",
		"host    replication     all             ::/0                    reject",
		skylexHBAEnd,
		"",
	}, "\n")
	content = removeManagedHBAPrefix(content)
	return managed + strings.TrimLeft(content, "\n")
}

func (p *Instance) refreshManagedHBAPrefix(ctx context.Context, reload bool) error {
	changed, err := p.ensureManagedHBAPrefixCurrent()
	if err != nil {
		return err
	}
	if !changed || !reload {
		return nil
	}
	if err := p.Reload(ctx); err != nil {
		return fmt.Errorf("reload after managed hba prefix refresh: %w", err)
	}
	return nil
}

func (p *Instance) ensureManagedHBAPrefixCurrent() (bool, error) {
	mainPath := filepath.Join(p.DataDir, "pg_hba.conf")
	previousMain, err := os.ReadFile(mainPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pg_hba.conf: %w", err)
	}
	if !strings.Contains(string(previousMain), skylexHBABegin) {
		return false, nil
	}

	nextMain := []byte(withManagedHBAPrefix(string(previousMain), p.Superuser))
	if bytes.Equal(previousMain, nextMain) {
		return false, nil
	}
	if err := os.WriteFile(mainPath, nextMain, 0600); err != nil {
		return false, fmt.Errorf("write pg_hba.conf managed prefix: %w", err)
	}
	return true, nil
}

func removeManagedHBAPrefix(content string) string {
	begin := strings.Index(content, skylexHBABegin)
	if begin < 0 {
		return content
	}
	end := strings.Index(content[begin:], skylexHBAEnd)
	if end < 0 {
		return content
	}
	end += begin + len(skylexHBAEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[:begin] + content[end:]
}

func restoreHBAInclude(path string, previous []byte, existed bool) {
	if existed {
		_ = os.WriteFile(path, previous, 0600)
		return
	}
	_ = os.Remove(path)
}

func canonicalStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func hbaField(value string) string {
	if value == "" {
		return "all"
	}
	return value
}

func hbaList(values []string) string {
	items := canonicalStrings(values)
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, ",")
}
