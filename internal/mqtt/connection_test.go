package mqtt

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

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

// generateTestCertificates
func generateTestCertificates(t *testing.T) (certFile, keyFile, caFile string, cleanup func()) {
	tempDir := t.TempDir()

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	assert.NoError(t, err)

	caFile = filepath.Join(tempDir, "ca.pem")
	caOut, err := os.Create(caFile)
	assert.NoError(t, err)
	err = pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	assert.NoError(t, err)
	caOut.Close()

	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "Test Client",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	assert.NoError(t, err)

	certFile = filepath.Join(tempDir, "client-cert.pem")
	certOut, err := os.Create(certFile)
	assert.NoError(t, err)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	assert.NoError(t, err)
	certOut.Close()

	keyFile = filepath.Join(tempDir, "client-key.pem")
	keyOut, err := os.Create(keyFile)
	assert.NoError(t, err)
	err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientPrivKey)})
	assert.NoError(t, err)
	keyOut.Close()

	cleanup = func() {
		// do nothing, no cleanup needed
	}

	return certFile, keyFile, caFile, cleanup
}

func TestSetNewClientFunc(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	manager := NewConnectionManager(options, 60)

	// Test default newClientFunc
	assert.NotNil(t, manager.optionsCtx.newClientFunc, "Default newClientFunc should not be nil")

	// Set custom newClientFunc
	manager.SetNewClientFunc(mockNewClientFunc)

	// Verify the newClientFunc was changed
	assert.NotNil(t, manager.optionsCtx.newClientFunc, "Custom newClientFunc should not be nil")

	// Create a client using the mock function
	client := manager.optionsCtx.newClientFunc(mqtt.NewClientOptions())
	assert.NotNil(t, client, "Client created with mock function should not be nil")

	// Test the mock client behavior
	mockClient, ok := client.(*mockMQTTClient)
	assert.True(t, ok, "Client should be of type mockMQTTClient")
	assert.False(t, mockClient.IsConnected(), "New client should not be connected")

	mockClient.Connect()
	assert.True(t, mockClient.IsConnected(), "Client should be connected after Connect()")
}

func TestRunConnections(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// Run connections
	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections should not return error")

	// Wait for a short time to allow all connections to be established
	time.Sleep(100 * time.Millisecond)

	// Verify that clients were created and connected
	manager.clientsMutex.Lock()
	assert.Equal(t, 2, len(manager.activeClients), "Should have created 2 clients")
	for i, client := range manager.activeClients {
		mockClient, ok := client.(*mockMQTTClient)
		assert.True(t, ok, "Client %d should be mockMQTTClient", i)
		assert.True(t, mockClient.IsConnected(), "Client %d should be connected", i)
	}
	manager.clientsMutex.Unlock()
}




func TestKeepConnections(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// Run connections first
	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections should not return error")

	// Wait for a short time to allow all connections to be established
	time.Sleep(100 * time.Millisecond)

	// Test keep connections with timeout
	done := make(chan struct{})
	go func() {
		err := manager.KeepConnections()
		assert.NoError(t, err, "KeepConnections should not return error")
		close(done)
	}()

	// Wait for keep connections to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("KeepConnections did not finish within expected time")
	}
}

func TestDisconnectAll(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// Run connections first
	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections should not return error")

	// Wait for a short time to allow all connections to be established
	time.Sleep(100 * time.Millisecond)

	// Verify clients are connected
	manager.clientsMutex.Lock()
	assert.Equal(t, 2, len(manager.activeClients), "Should have 2 active clients")
	manager.clientsMutex.Unlock()

	// Disconnect all clients
	err = manager.DisconnectAll()
	assert.NoError(t, err, "DisconnectAll should not return error")

	// Verify all clients are disconnected
	manager.clientsMutex.Lock()
	for i, client := range manager.activeClients {
		mockClient, ok := client.(*mockMQTTClient)
		assert.True(t, ok, "Client %d should be mockMQTTClient", i)
		assert.False(t, mockClient.IsConnected(), "Client %d should be disconnected", i)
		assert.True(t, mockClient.disconnected, "Client %d should have Disconnect() called", i)
	}
	manager.clientsMutex.Unlock()
}

func TestTLSConnection(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// 生成测试证书
	certFile, keyFile, caFile, cleanup := generateTestCertificates(t)
	defer cleanup()

	// 设置 TLS 配置
	options.ClientCertFile = certFile
	options.ClientKeyFile = keyFile
	options.CaCertFile = caFile
	options.Servers = []string{"tls://localhost:8883"}

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// 运行连接
	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections with TLS should not return error")

	// 验证客户端是否已连接
	manager.clientsMutex.Lock()
	assert.Equal(t, 2, len(manager.activeClients), "Should have created 2 TLS clients")
	for i, client := range manager.activeClients {
		mockClient, ok := client.(*mockMQTTClient)
		assert.True(t, ok, "Client %d should be mockMQTTClient", i)
		assert.True(t, mockClient.IsConnected(), "TLS Client %d should be connected", i)
	}
	manager.clientsMutex.Unlock()
}

func TestInvalidTLSCertificates(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	tempDir := t.TempDir()
	options.ClientCertFile = filepath.Join(tempDir, "non-existent-cert.pem")
	options.ClientKeyFile = filepath.Join(tempDir, "non-existent-key.pem")
	options.CaCertFile = filepath.Join(tempDir, "non-existent-ca.pem")
	options.Servers = []string{"tls://localhost:8883"}

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	err := manager.RunConnections()
	assert.Error(t, err, "RunConnections with invalid TLS certificates should return error")
}

func TestInsecureSkipVerifyTLS(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	options.SkipVerify = true
	options.Servers = []string{"tls://localhost:8883"}

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections with skip verify should not return error")

	manager.clientsMutex.Lock()
	assert.Equal(t, 2, len(manager.activeClients), "Should have created 2 clients with skip verify")
	for i, client := range manager.activeClients {
		mockClient, ok := client.(*mockMQTTClient)
		assert.True(t, ok, "Client %d should be mockMQTTClient", i)
		assert.True(t, mockClient.IsConnected(), "Client %d with skip verify should be connected", i)
	}
	manager.clientsMutex.Unlock()
}

func TestGenerateClientCertificate(t *testing.T) {
	// Create a temporary directory for test certificates
	tempDir := t.TempDir()

	// Create test CA key and certificate files
	caKeyFile := tempDir + "/ca.key"
	caCertFile := tempDir + "/ca.pem"

	// Generate test CA key and certificate
	err := generateTestCACertificate(caKeyFile, caCertFile)
	assert.NoError(t, err, "Failed to generate test CA certificate")

	// Test certificate generation
	clientID := "test-client-1"
	certPEM, keyPEM, err := generateClientCertificate(caKeyFile, caCertFile, clientID)
	assert.NoError(t, err, "Failed to generate client certificate")
	assert.NotNil(t, certPEM, "Certificate PEM should not be nil")
	assert.NotNil(t, keyPEM, "Key PEM should not be nil")

	// Write the generated certificates to files for verification
	clientKeyFile := tempDir + "/client.key"
	clientCertFile := tempDir + "/client.pem"
	err = os.WriteFile(clientKeyFile, keyPEM, 0600)
	assert.NoError(t, err, "Failed to write client key")
	err = os.WriteFile(clientCertFile, certPEM, 0600)
	assert.NoError(t, err, "Failed to write client certificate")

	// Verify the generated files exist and are valid certificates
	_, err = os.Stat(clientKeyFile)
	assert.NoError(t, err, "Client key file should exist")
	_, err = os.Stat(clientCertFile)
	assert.NoError(t, err, "Client certificate file should exist")

	// Test error cases
	tests := []struct {
		name     string
		caKey    string
		caCert   string
		clientID string
	}{
		{
			name:     "Invalid CA key file",
			caKey:    "nonexistent.key",
			caCert:   caCertFile,
			clientID: "test-client",
		},
		{
			name:     "Invalid CA cert file",
			caKey:    caKeyFile,
			caCert:   "nonexistent.pem",
			clientID: "test-client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPEM, keyPEM, err := generateClientCertificate(tt.caKey, tt.caCert, tt.clientID)
			assert.Error(t, err, "Should fail with invalid files")
			assert.Nil(t, certPEM, "Certificate PEM should be nil")
			assert.Nil(t, keyPEM, "Key PEM should be nil")
		})
	}
}

func generateTestCACertificate(caKeyFile, caCertFile string) error {
	// Generate CA private key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return err
	}

	// Save CA private key
	caKeyFileHandle, err := os.Create(caKeyFile)
	if err != nil {
		return err
	}
	defer caKeyFileHandle.Close()

	err = pem.Encode(caKeyFileHandle, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
	if err != nil {
		return err
	}

	// Save CA certificate
	caCertFileHandle, err := os.Create(caCertFile)
	if err != nil {
		return err
	}
	defer caCertFileHandle.Close()

	return pem.Encode(caCertFileHandle, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})
}
