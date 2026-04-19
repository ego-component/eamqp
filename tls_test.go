package eamqp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_BuildTLSConfigLoadsCACert(t *testing.T) {
	caFile := writeTestCACert(t)
	cfg := Config{
		TLSCACert:     caFile,
		TLSServerName: "rabbit.local",
	}

	tlsConfig, err := cfg.buildTLSConfig("fallback.local")
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	require.NotNil(t, tlsConfig.RootCAs)

	assert.Equal(t, "rabbit.local", tlsConfig.ServerName)
	assert.NotEmpty(t, tlsConfig.RootCAs.Subjects())
}

func TestTLSConfig_LoadTLSConfigLoadsCACert(t *testing.T) {
	caFile := writeTestCACert(t)
	cfg := &TLSConfig{CACert: caFile}

	tlsConfig, err := cfg.LoadTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	require.NotNil(t, tlsConfig.RootCAs)

	assert.NotEmpty(t, tlsConfig.RootCAs.Subjects())
}

func TestTLSConfig_LoadTLSConfigRequiresCertAndKeyTogether(t *testing.T) {
	cfg := &TLSConfig{CertFile: "client.crt"}

	_, err := cfg.LoadTLSConfig()

	assert.Error(t, err)
}

func writeTestCACert(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "eamqp-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NotEmpty(t, pemBytes)
	require.NoError(t, os.WriteFile(caFile, pemBytes, 0o600))

	return caFile
}
