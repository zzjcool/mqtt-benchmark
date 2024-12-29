package mqtt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCertificates 生成用于测试的 TLS 证书
func generateTestCertificates(t *testing.T) (certFile, keyFile, caFile string, cleanup func()) {
	// 创建临时目录
	tempDir := t.TempDir()

	// 生成 CA 私钥
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 创建 CA 证书模板
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

	// 创建 CA 证书
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// 保存 CA 证书
	caFile = filepath.Join(tempDir, "ca.pem")
	caOut, err := os.Create(caFile)
	require.NoError(t, err)
	err = pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	require.NoError(t, err)
	caOut.Close()

	// 生成客户端私钥
	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 创建客户端证书模板
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

	// 创建客户端证书
	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// 保存客户端证书
	certFile = filepath.Join(tempDir, "client-cert.pem")
	certOut, err := os.Create(certFile)
	require.NoError(t, err)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: clientBytes})
	require.NoError(t, err)
	certOut.Close()

	// 保存客户端私钥
	keyFile = filepath.Join(tempDir, "client-key.pem")
	keyOut, err := os.Create(keyFile)
	require.NoError(t, err)
	err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientPrivKey)})
	require.NoError(t, err)
	keyOut.Close()

	cleanup = func() {
		// 使用 t.TempDir() 会自动清理临时目录，所以这里不需要手动清理
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

func TestRunConnectionsWithError(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()
	
	// Set a short connection timeout
	options.ConnectTimeout = 1 // 1 second timeout
	
	manager := NewConnectionManager(options, 1)
	expectedErr := errors.New("connection failed")
	manager.SetNewClientFunc(mockNewClientFuncWithError(expectedErr))

	// Run connections
	err := manager.RunConnections()
	assert.Error(t, err, "RunConnections should return error on connection failure")


	// Wait for a short time to allow all connection attempts to complete
	time.Sleep(100 * time.Millisecond)

	// Verify that no clients were connected
	manager.clientsMutex.Lock()
	assert.Equal(t, 0, len(manager.activeClients), "Should have no active clients")
	manager.clientsMutex.Unlock()
}

func TestRunConnectionsWithTimeout(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()
	
	// Set a very short connection timeout
	options.ConnectTimeout = 1
	
	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFuncWithDelay(2 * time.Second))

	// Run connections
	err := manager.RunConnections()
	assert.Error(t, err, "RunConnections should return error on timeout")

	// Wait for a short time to allow all connection attempts to complete
	time.Sleep(100 * time.Millisecond)

	// Verify that no clients were connected
	manager.clientsMutex.Lock()
	assert.Equal(t, 0, len(manager.activeClients), "Should have no active clients")
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
	options.CertFile = certFile
	options.KeyFile = keyFile
	options.CaFile = caFile
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

	// 使用临时目录中的无效证书路径
	tempDir := t.TempDir()
	options.CertFile = filepath.Join(tempDir, "non-existent-cert.pem")
	options.KeyFile = filepath.Join(tempDir, "non-existent-key.pem")
	options.CaFile = filepath.Join(tempDir, "non-existent-ca.pem")
	options.Servers = []string{"tls://localhost:8883"}

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// 运行连接
	err := manager.RunConnections()
	assert.Error(t, err, "RunConnections with invalid TLS certificates should return error")
}

func TestInsecureSkipVerifyTLS(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// 设置跳过证书验证
	options.SkipVerify = true
	options.Servers = []string{"tls://localhost:8883"}

	manager := NewConnectionManager(options, 1)
	manager.SetNewClientFunc(mockNewClientFunc)

	// 运行连接
	err := manager.RunConnections()
	assert.NoError(t, err, "RunConnections with skip verify should not return error")

	// 验证客户端是否已连接
	manager.clientsMutex.Lock()
	assert.Equal(t, 2, len(manager.activeClients), "Should have created 2 clients with skip verify")
	for i, client := range manager.activeClients {
		mockClient, ok := client.(*mockMQTTClient)
		assert.True(t, ok, "Client %d should be mockMQTTClient", i)
		assert.True(t, mockClient.IsConnected(), "Client %d with skip verify should be connected", i)
	}
	manager.clientsMutex.Unlock()
}
