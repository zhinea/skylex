package agent

import "testing"

func TestParseReplicationTarget(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		defaultPort int
		wantHost    string
		wantPort    int
	}{
		{
			name:        "host:port parsed correctly (bug regression)",
			payload:     "localhost:5433",
			defaultPort: 5434,
			wantHost:    "localhost",
			wantPort:    5433,
		},
		{
			name:        "no port uses default",
			payload:     "localhost",
			defaultPort: 5432,
			wantHost:    "localhost",
			wantPort:    5432,
		},
		{
			name:        "empty payload uses default port",
			payload:     "",
			defaultPort: 5432,
			wantHost:    "",
			wantPort:    5432,
		},
		{
			name:        "fqdn with port",
			payload:     "db.example.com:5432",
			defaultPort: 5432,
			wantHost:    "db.example.com",
			wantPort:    5432,
		},
		{
			name:        "non-numeric port falls back to default",
			payload:     "host:notanumber",
			defaultPort: 5432,
			wantHost:    "host",
			wantPort:    5432,
		},
		{
			name:        "extra colon in payload — SplitN(2) gives port segment with extra data",
			payload:     "host:5432:extra",
			defaultPort: 5432,
			wantHost:    "host",
			wantPort:    5432,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, port := parseReplicationTarget(tc.payload, tc.defaultPort)
			if host != tc.wantHost {
				t.Errorf("host = %q, want %q", host, tc.wantHost)
			}
			if port != tc.wantPort {
				t.Errorf("port = %d, want %d", port, tc.wantPort)
			}
		})
	}
}
