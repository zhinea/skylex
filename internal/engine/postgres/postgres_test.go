package postgres

import (
	"strings"
	"testing"

	"github.com/zhinea/skylex/internal/engine"
)

func TestProviderImplementsExtensionCatalog(t *testing.T) {
	var p interface{} = Provider{}
	if _, ok := p.(engine.ExtensionCatalog); !ok {
		t.Fatal("postgres.Provider must implement engine.ExtensionCatalog")
	}
}

func TestAvailableExtensionsNonEmptyAndValid(t *testing.T) {
	exts := Provider{}.AvailableExtensions()
	if len(exts) == 0 {
		t.Fatal("expected a non-empty extension catalog")
	}
	seen := map[string]bool{}
	for _, e := range exts {
		if e.Name == "" || e.Label == "" || e.Description == "" {
			t.Fatalf("extension %+v has empty fields", e)
		}
		if seen[e.Name] {
			t.Fatalf("duplicate extension name %q in catalog", e.Name)
		}
		seen[e.Name] = true
		// Every advertised extension must validate.
		if err := (Provider{}).ValidateExtensionName(e.Name); err != nil {
			t.Fatalf("advertised extension %q failed validation: %v", e.Name, err)
		}
	}
}

func TestValidateExtensionNameRejectsUnknownAndUnsafe(t *testing.T) {
	cases := []string{
		"",                       // empty
		"drop_all",               // not in allowlist
		"pg_trgm; DROP TABLE x",  // injection attempt
		"uuid ossp",              // space
		"pg_stat_statements",     // real extension, but excluded (needs restart)
		strings.Repeat("a", 100), // too long
	}
	for _, name := range cases {
		if err := (Provider{}).ValidateExtensionName(name); err == nil {
			t.Fatalf("expected extension name %q to be rejected", name)
		}
	}
}

func TestActionMapsApplyExtensions(t *testing.T) {
	action, ok := Provider{}.Action(engine.OpApplyExtensions)
	if !ok || action != "pg_apply_extensions" {
		t.Fatalf("expected pg_apply_extensions action, got %q (ok=%v)", action, ok)
	}
}
