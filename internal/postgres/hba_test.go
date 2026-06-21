package postgres

import (
	"os"
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
	first := withManagedHBAPrefix("host all all 0.0.0.0/0 scram-sha-256\n", "postgres")
	second := withManagedHBAPrefix(first, "postgres")
	if strings.Count(second, skylexHBABegin) != 1 {
		t.Fatalf("expected one managed block, got:\n%s", second)
	}
	if !strings.Contains(second, "host    all             all             0.0.0.0/0               reject") {
		t.Fatalf("expected broad reject before legacy content:\n%s", second)
	}
}

func TestWithManagedHBAPrefixUsesHBAIncludeSyntax(t *testing.T) {
	conf := withManagedHBAPrefix("host all all 127.0.0.1/32 scram-sha-256\n", "postgres")
	if !strings.Contains(conf, "include_if_exists "+skylexHBAFileName+"\n") {
		t.Fatalf("expected pg_hba.conf include syntax without single quotes:\n%s", conf)
	}
	if strings.Contains(conf, "'"+skylexHBAFileName+"'") {
		t.Fatalf("pg_hba.conf treats single quotes as filename characters:\n%s", conf)
	}
}

func TestWithManagedHBAPrefixAllowsLocalBeforeBroadReject(t *testing.T) {
	conf := withManagedHBAPrefix("host all all 0.0.0.0/0 scram-sha-256\n", "postgres")
	local := strings.Index(conf, "host    all             postgres")
	reject := strings.Index(conf, "host    all             all             0.0.0.0/0               reject")
	if local < 0 || reject < 0 || local > reject {
		t.Fatalf("expected local TCP admin access before broad reject:\n%s", conf)
	}
}

func TestEnsureManagedHBAPrefixCurrentMigratesLegacyQuotedInclude(t *testing.T) {
	dir := t.TempDir()
	legacy := strings.Join([]string{
		skylexHBABegin,
		"include_if_exists '" + skylexHBAFileName + "'",
		"host    all             all             0.0.0.0/0               reject",
		"host    all             all             ::/0                    reject",
		"host    replication     all             0.0.0.0/0               reject",
		"host    replication     all             ::/0                    reject",
		skylexHBAEnd,
		"host all all 0.0.0.0/0 scram-sha-256",
	}, "\n")
	if err := os.WriteFile(dir+"/pg_hba.conf", []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy pg_hba.conf: %v", err)
	}

	changed, err := (&Instance{DataDir: dir, Superuser: "postgres"}).ensureManagedHBAPrefixCurrent()
	if err != nil {
		t.Fatalf("refresh managed hba prefix: %v", err)
	}
	if !changed {
		t.Fatal("expected legacy managed HBA prefix to be rewritten")
	}
	data, err := os.ReadFile(dir + "/pg_hba.conf")
	if err != nil {
		t.Fatalf("read rewritten pg_hba.conf: %v", err)
	}
	conf := string(data)
	if strings.Contains(conf, "'"+skylexHBAFileName+"'") {
		t.Fatalf("expected quoted include to be removed:\n%s", conf)
	}
	if !strings.Contains(conf, "include_if_exists "+skylexHBAFileName+"\n") {
		t.Fatalf("expected unquoted include:\n%s", conf)
	}
	local := strings.Index(conf, "host    all             postgres")
	reject := strings.Index(conf, "host    all             all             0.0.0.0/0               reject")
	if local < 0 || reject < 0 || local > reject {
		t.Fatalf("expected local TCP admin access before broad reject:\n%s", conf)
	}
}
