package postgres

import (
	"strings"
	"testing"
)

func TestRenderHBAPolicyUsesExplicitApplicationRolesAndDatabases(t *testing.T) {
	conf := RenderHBAPolicy(HBAPolicy{
		AdminCIDRs:       []string{"10.0.0.0/24"},
		ReplicationCIDRs: []string{"10.0.1.0/24"},
		ApplicationCIDRs: []string{"10.0.2.0/24"},
		ApplicationRoles: []string{"app_user"},
		ApplicationDBs:   []string{"app_db"},
	}, "postgres", "replicator")

	for _, want := range []string{
		"host    all             postgres             10.0.0.0/24",
		"host    replication     replicator             10.0.1.0/24",
		"host    app_db             app_user             10.0.2.0/24",
		"127.0.0.1/32",
		"::1/128",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("rendered HBA missing %q:\n%s", want, conf)
		}
	}
}

func TestWithManagedHBAPrefixReplacesPreviousBlock(t *testing.T) {
	first := withManagedHBAPrefix("host all all 0.0.0.0/0 scram-sha-256\n")
	second := withManagedHBAPrefix(first)
	if strings.Count(second, skylexHBABegin) != 1 {
		t.Fatalf("expected one managed block, got:\n%s", second)
	}
	if !strings.Contains(second, "host    all             all             0.0.0.0/0               reject") {
		t.Fatalf("expected broad reject before legacy content:\n%s", second)
	}
}
