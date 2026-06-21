package postgres

import (
	"os"
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
