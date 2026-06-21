package postgres

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTLSConfigSettingsPreservesExistingSettings(t *testing.T) {
	tmp := t.TempDir()
	includePath := tmp + "/skylex.conf.include"
	if err := os.WriteFile(includePath, []byte("work_mem = 16MB\nssl = off\nssl_ca_file = '/old/ca.crt'\n"), 0600); err != nil {
		t.Fatalf("write include: %v", err)
	}

	settings := (TLSConfig{Mode: TLSModeRequired, CertFile: "/cert.crt", KeyFile: "/key.key"}).Settings(includePath)
	if settings["work_mem"] != "16MB" {
		t.Fatalf("expected existing work_mem to be preserved, got %#v", settings)
	}
	if settings["ssl"] != "on" || settings["ssl_cert_file"] != "'/cert.crt'" || settings["ssl_key_file"] != "'/key.key'" {
		t.Fatalf("expected TLS settings to be applied, got %#v", settings)
	}
	if _, ok := settings["ssl_ca_file"]; ok {
		t.Fatalf("expected ssl_ca_file to be removed when omitted, got %#v", settings)
	}
}

func TestTLSConfigEnsureManagedCertificate(t *testing.T) {
	dir := t.TempDir()
	cfg, managed, err := (TLSConfig{Mode: TLSModeRequired, Hosts: []string{"pg.example.test", "127.0.0.1"}}).resolved(dir)
	if err != nil {
		t.Fatalf("resolve managed cert paths: %v", err)
	}
	if !managed {
		t.Fatal("expected managed certificate mode")
	}
	if err := cfg.ensureManagedCertificate(); err != nil {
		t.Fatalf("ensure managed cert: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, managedTLSDirName, managedTLSCertName)); err != nil {
		t.Fatalf("expected certificate file: %v", err)
	}
	keyInfo, err := os.Stat(filepath.Join(dir, managedTLSDirName, managedTLSKeyName))
	if err != nil {
		t.Fatalf("expected key file: %v", err)
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Fatalf("expected key mode 0600, got %v", keyInfo.Mode().Perm())
	}
	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("expected PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if len(cert.DNSNames) == 0 || cert.DNSNames[0] != "pg.example.test" {
		t.Fatalf("expected DNS SAN pg.example.test, got %#v", cert.DNSNames)
	}
}

func TestPGCmdDockerRunsInsideContainerWithPassword(t *testing.T) {
	p := New("/data", "", "16", 5432, "postgres", "replicator", "secret", nil)
	p.UseDocker("postgres:16", "skylex-postgres", "", "postgres")

	cmd := p.pgCmd(context.Background(), "psql", "-h", "127.0.0.1", "-c", "SHOW ssl")
	want := []string{
		"docker", "exec", "-u", "postgres", "-e", "PGDATA=/var/lib/postgresql/data",
		"-e", "PGPASSWORD=secret", "skylex-postgres", "psql", "-h", "127.0.0.1", "-c", "SHOW ssl",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("unexpected docker pg command args:\nwant %#v\n got %#v", want, cmd.Args)
	}
}
