package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

func generateClusterCA(clusterID string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return "", "", fmt.Errorf("generate ca key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("Skylex PostgreSQL CA %s", clusterID),
		},
		NotBefore:             time.Now().UTC().Add(-5 * time.Minute),
		NotAfter:              time.Now().UTC().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("create ca certificate: %w", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM, nil
}

func generateServerCertificate(caCertPEM, caKeyPEM string, hosts []string) (string, string, error) {
	caBlock, _ := pem.Decode([]byte(caCertPEM))
	if caBlock == nil {
		return "", "", fmt.Errorf("decode ca certificate")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse ca certificate: %w", err)
	}
	keyBlock, _ := pem.Decode([]byte(caKeyPEM))
	if keyBlock == nil {
		return "", "", fmt.Errorf("decode ca key")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse ca key: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return "", "", fmt.Errorf("generate server key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return "", "", err
	}
	canonicalHosts := canonicalCertHosts(hosts)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: firstCertHost(canonicalHosts),
		},
		NotBefore:             time.Now().UTC().Add(-5 * time.Minute),
		NotAfter:              time.Now().UTC().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range canonicalHosts {
		if ip := net.ParseIP(host); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			continue
		}
		tmpl.DNSNames = append(tmpl.DNSNames, host)
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create server certificate: %w", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}))
	return certPEM, keyPEM, nil
}

func randomSerial() (*big.Int, error) {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return serial, nil
}

func canonicalCertHosts(hosts []string) []string {
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

func firstCertHost(hosts []string) string {
	for _, host := range hosts {
		return host
	}
	return "localhost"
}
