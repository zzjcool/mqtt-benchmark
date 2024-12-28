package mqtt

import (
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

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
	assert.NoError(t, err, "RunConnections should not return error even if clients fail to connect")

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
	assert.NoError(t, err, "RunConnections should not return error on timeout")

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
