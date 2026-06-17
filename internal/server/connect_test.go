package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeVersion(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	srv.serveVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != versionString() {
		t.Fatalf("expected %q, got %q", versionString(), rec.Body.String())
	}
}

func TestServeAgentInstallScript(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rec := httptest.NewRecorder()

	srv.serveAgentInstallScript(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Skylex agent installer") {
		t.Fatal("install script missing expected content")
	}
	if strings.Contains(body, "@@VERSION@@") {
		t.Fatal("install script still contains unsubstituted version placeholder")
	}
}

func TestServeAgentInstallScriptRejectsPost(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/install.sh", nil)
	rec := httptest.NewRecorder()

	srv.serveAgentInstallScript(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}
