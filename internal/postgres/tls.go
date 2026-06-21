package postgres

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	TLSModeDisabled = "disabled"
	TLSModePrefer   = "prefer"
	TLSModeRequired = "required"
)

const (
	managedTLSDirName  = "skylex-tls"
	managedTLSCertName = "server.crt"
	managedTLSKeyName  = "server.key"
)

// TLSConfig describes either manual certificate paths or Skylex-managed cert generation.
type TLSConfig struct {
	Mode     string
	CertFile string
	KeyFile  string
	CAFile   string
	CertPEM  string
	KeyPEM   string
	CAPEM    string
	Hosts    []string
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
	if c.CertPEM != "" || c.KeyPEM != "" {
		if c.CertPEM == "" || c.KeyPEM == "" {
			return fmt.Errorf("certificate and key secrets must both be present")
		}
		return nil
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("cert_file and key_file must both be set for manual TLS certificates, or both left empty for Skylex-managed certificates")
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

func (c TLSConfig) resolved(dataDir string) (TLSConfig, bool, error) {
	if c.Mode == TLSModeDisabled || (c.CertFile != "" && c.KeyFile != "") {
		return c, false, nil
	}
	managedDir := filepath.Join(dataDir, managedTLSDirName)
	resolved := c
	resolved.CertFile = filepath.Join(managedDir, managedTLSCertName)
	resolved.KeyFile = filepath.Join(managedDir, managedTLSKeyName)
	if c.CAPEM != "" {
		resolved.CAFile = filepath.Join(managedDir, "ca.crt")
	}
	return resolved, true, nil
}

func (c TLSConfig) ensureManagedCertificate() error {
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("managed certificate paths are empty")
	}
	if c.CertPEM != "" || c.KeyPEM != "" {
		if c.CertPEM == "" || c.KeyPEM == "" {
			return fmt.Errorf("certificate and key secrets must both be present")
		}
		if err := os.MkdirAll(filepath.Dir(c.CertFile), 0700); err != nil {
			return fmt.Errorf("create managed tls directory: %w", err)
		}
		if err := os.WriteFile(c.CertFile, []byte(c.CertPEM), 0644); err != nil {
			return fmt.Errorf("write managed certificate: %w", err)
		}
		if err := os.WriteFile(c.KeyFile, []byte(c.KeyPEM), 0600); err != nil {
			return fmt.Errorf("write managed private key: %w", err)
		}
		if c.CAPEM != "" {
			caPath := filepath.Join(filepath.Dir(c.CertFile), "ca.crt")
			if err := os.WriteFile(caPath, []byte(c.CAPEM), 0644); err != nil {
				return fmt.Errorf("write managed ca certificate: %w", err)
			}
		}
		return nil
	}
	if _, certErr := os.Stat(c.CertFile); certErr == nil {
		if _, keyErr := os.Stat(c.KeyFile); keyErr == nil {
			return nil
		}
	} else if !os.IsNotExist(certErr) {
		return fmt.Errorf("stat managed certificate: %w", certErr)
	}
	if err := os.MkdirAll(filepath.Dir(c.CertFile), 0700); err != nil {
		return fmt.Errorf("create managed tls directory: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return fmt.Errorf("generate private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: firstCertificateHost(c.Hosts),
		},
		NotBefore:             time.Now().UTC().Add(-5 * time.Minute),
		NotAfter:              time.Now().UTC().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range canonicalCertificateHosts(c.Hosts) {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, host)
	}
	if len(template.DNSNames) == 0 && len(template.IPAddresses) == 0 {
		template.DNSNames = []string{"localhost"}
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(c.CertFile, certPEM, 0644); err != nil {
		return fmt.Errorf("write managed certificate: %w", err)
	}
	if err := os.WriteFile(c.KeyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("write managed private key: %w", err)
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

func firstCertificateHost(hosts []string) string {
	for _, host := range canonicalCertificateHosts(hosts) {
		return host
	}
	return "localhost"
}

func canonicalCertificateHosts(hosts []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(hosts)+1)
	for _, host := range hosts {
		h := strings.TrimSpace(host)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	if !seen["localhost"] {
		out = append(out, "localhost")
	}
	return out
}

func (p *Instance) verifyTLSActive(ctx context.Context, mode string) error {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-t", "-A",
		"-c", "SHOW ssl",
	)
	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("show ssl: %w\n%s", err, string(output))
	}
	ssl := strings.TrimSpace(string(output))
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
