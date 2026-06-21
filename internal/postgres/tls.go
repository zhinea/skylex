package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	TLSModeDisabled = "disabled"
	TLSModePrefer   = "prefer"
	TLSModeRequired = "required"
)

// TLSConfig describes certificate paths that already exist on the agent host.
type TLSConfig struct {
	Mode     string
	CertFile string
	KeyFile  string
	CAFile   string
}

func (c TLSConfig) Validate() error {
	switch c.Mode {
	case TLSModeDisabled, TLSModePrefer, TLSModeRequired:
	default:
		return fmt.Errorf("invalid TLS mode %q", c.Mode)
	}
	if c.Mode == TLSModeDisabled {
		return nil
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("cert_file and key_file are required when TLS is enabled")
	}
	for label, path := range map[string]string{"cert_file": c.CertFile, "key_file": c.KeyFile, "ca_file": c.CAFile} {
		if path == "" {
			continue
		}
		if strings.ContainsAny(path, "\x00\r\n") || !strings.HasPrefix(path, "/") {
			return fmt.Errorf("%s must be an absolute path without control characters", label)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%s %q is not readable: %w", label, path, err)
		}
	}
	return nil
}

func (c TLSConfig) Settings(existingPath string) map[string]string {
	settings := readSkylexIncludeSettings(existingPath)
	if c.Mode == TLSModeDisabled {
		delete(settings, "ssl_cert_file")
		delete(settings, "ssl_key_file")
		delete(settings, "ssl_ca_file")
		settings["ssl"] = "off"
		return settings
	}
	settings["ssl"] = "on"
	settings["ssl_cert_file"] = quoteConfigValue(c.CertFile)
	settings["ssl_key_file"] = quoteConfigValue(c.KeyFile)
	if c.CAFile != "" {
		settings["ssl_ca_file"] = quoteConfigValue(c.CAFile)
	} else {
		delete(settings, "ssl_ca_file")
	}
	return settings
}

func readSkylexIncludeSettings(path string) map[string]string {
	settings := map[string]string{}
	if path == "" {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		settings[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return settings
}

func quoteConfigValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (p *Instance) verifyTLSActive(ctx context.Context, mode string) error {
	conn, err := p.localConnect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var ssl string
	if err := conn.QueryRow(ctx, "SHOW ssl").Scan(&ssl); err != nil {
		return fmt.Errorf("show ssl: %w", redactPGError(err))
	}
	switch mode {
	case TLSModeDisabled:
		if strings.EqualFold(ssl, "off") {
			return nil
		}
		return fmt.Errorf("expected ssl=off, got %s", ssl)
	case TLSModePrefer, TLSModeRequired:
		if strings.EqualFold(ssl, "on") {
			return nil
		}
		return fmt.Errorf("expected ssl=on, got %s", ssl)
	default:
		return fmt.Errorf("invalid TLS mode %q", mode)
	}
}
