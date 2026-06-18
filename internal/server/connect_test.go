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
