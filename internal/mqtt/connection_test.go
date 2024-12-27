package mqtt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

// mockToken implements mqtt.Token interface for testing
type mockToken struct {
	err      error
	waitTime time.Duration
}

func (t *mockToken) Wait() bool                 { return true }
func (t *mockToken) WaitTimeout(d time.Duration) bool {
	if t.waitTime > d {
		return false
	}
	time.Sleep(t.waitTime)
	return true
}
func (t *mockToken) Error() error              { return t.err }
func (t *mockToken) Done() <-chan struct{}     { return nil }

// mockMQTTClient implements mqtt.Client interface for testing
type mockMQTTClient struct {
	mqtt.Client
	connected    bool
	disconnected bool
	mu           sync.Mutex
	opts         *mqtt.ClientOptions
	connectErr   error
	connectDelay time.Duration
}

func (m *mockMQTTClient) Connect() mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.connectDelay > 0 {
		return &mockToken{waitTime: m.connectDelay}
	}
	
	if m.connectErr != nil {
		return &mockToken{err: m.connectErr}
	}
	
	m.connected = true
	return &mockToken{}
}

func (m *mockMQTTClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockMQTTClient) Disconnect(quiesce uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.disconnected = true
}

func (m *mockMQTTClient) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.NewOptionsReader(m.opts)
}

// mockNewClientFunc is a test implementation of NewClientFunc
func mockNewClientFunc(opts *mqtt.ClientOptions) mqtt.Client {
	return &mockMQTTClient{
		opts: opts,
	}
}

// mockNewClientFuncWithError returns a client that will fail to connect
func mockNewClientFuncWithError(err error) NewClientFunc {
	return func(opts *mqtt.ClientOptions) mqtt.Client {
		return &mockMQTTClient{
			opts:       opts,
			connectErr: err,
		}
	}
}

// mockNewClientFuncWithDelay returns a client that will delay before connecting
func mockNewClientFuncWithDelay(delay time.Duration) NewClientFunc {
	return func(opts *mqtt.ClientOptions) mqtt.Client {
		return &mockMQTTClient{
			opts:         opts,
			connectDelay: delay,
		}
	}
}

// setupTest prepares a test environment
func setupTest(t *testing.T) (*OptionsCtx, context.CancelFunc) {
	// Initialize logger
	logger.InitLogger("debug")

	// Initialize metrics
	reg := prometheus.NewRegistry()
	metrics.MQTTConnectionRateLimit = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_connection_rate_limit",
		Help: "MQTT connection rate limit",
	})
	metrics.MQTTConnections = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mqtt_connections",
		Help: "Number of active MQTT connections",
	}, []string{"broker"})
	metrics.MQTTConnectionAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_connection_attempts",
		Help: "Number of MQTT connection attempts",
	}, []string{"broker", "result"})
	metrics.MQTTConnectionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_connection_errors",
		Help: "Number of MQTT connection errors",
	}, []string{"broker", "type"})
	metrics.MQTTConnectionTime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "mqtt_connection_time",
		Help: "MQTT connection time",
	})
	metrics.MQTTNewConnections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_new_connections",
		Help: "Number of new MQTT connections",
	}, []string{"broker"})

	reg.MustRegister(
		metrics.MQTTConnectionRateLimit,
		metrics.MQTTConnections,
		metrics.MQTTConnectionAttempts,
		metrics.MQTTConnectionErrors,
		metrics.MQTTConnectionTime,
		metrics.MQTTNewConnections,
	)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	
	options := &OptionsCtx{
		Context:          ctx,
		CancelFunc:       cancel,
		ConnRate:         100,
		ClientNum:        2,
		ClientPrefix:     "test-client",
		Servers:          []string{"tcp://localhost:1883"},
		ConnectTimeout:   5,
		KeepAliveSeconds: 60,
	}

	return options, cancel
}

func TestSetNewClientFunc(t *testing.T) {
	options, cancel := setupTest(t)
	defer cancel()

	manager := NewConnectionManager(options, 60)

	// Test default newClientFunc
	assert.NotNil(t, manager.newClientFunc, "Default newClientFunc should not be nil")

	// Set custom newClientFunc
	manager.SetNewClientFunc(mockNewClientFunc)

	// Verify the newClientFunc was changed
	assert.NotNil(t, manager.newClientFunc, "Custom newClientFunc should not be nil")
	
	// Create a client using the mock function
	client := manager.newClientFunc(mqtt.NewClientOptions())
	assert.NotNil(t, client, "Client created with mock function should not be nil")
	
	// Test the mock client behavior
	mockClient, ok := client.(*mockMQTTClient)
	assert.True(t, ok, "Client should be of type mockMQTTClient")
	assert.False(t, mockClient.IsConnected(), "New client should not be connected")
	
	mockClient.Connect()
	assert.True(t, mockClient.IsConnected(), "Client should be connected after Connect()")
}

func TestRunConnections(t *testing.T) {
	options, cancel := setupTest(t)
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
	options, cancel := setupTest(t)
	defer cancel()
	
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
	options, cancel := setupTest(t)
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
	options, cancel := setupTest(t)
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
	options, cancel := setupTest(t)
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
