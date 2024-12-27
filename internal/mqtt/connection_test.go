package mqtt

import (
	"context"
	"testing"
	"time"
	"net/url"
	"crypto/tls"
	"net/http"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

// mockClientOptionsReader implements mqtt.ClientOptionsReader interface for testing
type mockClientOptionsReader struct {
	broker   string
	clientID string
}

func (m *mockClientOptionsReader) Servers() []*url.URL {
	server, _ := url.Parse(m.broker)
	return []*url.URL{server}
}

func (m *mockClientOptionsReader) ClientID() string                       { return m.clientID }
func (m *mockClientOptionsReader) Username() string                       { return "" }
func (m *mockClientOptionsReader) Password() string                       { return "" }
func (m *mockClientOptionsReader) CleanSession() bool                    { return true }
func (m *mockClientOptionsReader) Order() bool                           { return true }
func (m *mockClientOptionsReader) WillEnabled() bool                     { return false }
func (m *mockClientOptionsReader) WillTopic() string                     { return "" }
func (m *mockClientOptionsReader) WillPayload() []byte                   { return nil }
func (m *mockClientOptionsReader) WillQos() byte                        { return 0 }
func (m *mockClientOptionsReader) WillRetained() bool                   { return false }
func (m *mockClientOptionsReader) ProtocolVersion() uint                { return 4 }
func (m *mockClientOptionsReader) TLSConfig() *tls.Config              { return nil }
func (m *mockClientOptionsReader) KeepAlive() time.Duration            { return 60 * time.Second }
func (m *mockClientOptionsReader) PingTimeout() time.Duration          { return 10 * time.Second }
func (m *mockClientOptionsReader) ConnectTimeout() time.Duration       { return 30 * time.Second }
func (m *mockClientOptionsReader) MaxReconnectInterval() time.Duration { return 10 * time.Minute }
func (m *mockClientOptionsReader) AutoReconnect() bool                 { return true }
func (m *mockClientOptionsReader) WriteTimeout() time.Duration         { return 0 }
func (m *mockClientOptionsReader) MessageChannelDepth() uint           { return 100 }
func (m *mockClientOptionsReader) HTTPHeaders() http.Header            { return nil }
func (m *mockClientOptionsReader) WebsocketOptions() *mqtt.WebsocketOptions { return nil }
func (m *mockClientOptionsReader) ConnectRetry() bool                  { return true }
func (m *mockClientOptionsReader) ConnectRetryInterval() time.Duration { return 10 * time.Second }
func (m *mockClientOptionsReader) ResumeSubs() bool                    { return true }

// mockMQTTClient implements mqtt.Client interface for testing
type mockMQTTClient struct {
	connected bool
	opts      *mqtt.ClientOptions
}

func newMockMQTTClient() *mockMQTTClient {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("test-client")
	return &mockMQTTClient{
		connected: false,
		opts:      opts,
	}
}

func (m *mockMQTTClient) Connect() mqtt.Token {
	m.connected = true
	return newMockToken()
}
func (m *mockMQTTClient) Disconnect(uint)                                    {}
func (m *mockMQTTClient) Publish(string, byte, bool, interface{}) mqtt.Token { return newMockToken() }
func (m *mockMQTTClient) Subscribe(string, byte, mqtt.MessageHandler) mqtt.Token {
	return newMockToken()
}
func (m *mockMQTTClient) SubscribeMultiple(map[string]byte, mqtt.MessageHandler) mqtt.Token {
	return newMockToken()
}
func (m *mockMQTTClient) Unsubscribe(...string) mqtt.Token        { return newMockToken() }
func (m *mockMQTTClient) AddRoute(string, mqtt.MessageHandler)    {}
func (m *mockMQTTClient) IsConnected() bool                       { return m.connected }
func (m *mockMQTTClient) IsConnectionOpen() bool                  { return m.connected }
func (m *mockMQTTClient) OptionsReader() mqtt.ClientOptionsReader { 
	return mqtt.NewOptionsReader(m.opts)
}

// mockToken implements mqtt.Token interface for testing
type mockToken struct {
	err error
	ch  chan struct{}
}

func newMockToken() *mockToken {
	return &mockToken{
		ch: make(chan struct{}, 1),
	}
}

func (m *mockToken) Wait() bool                     { return true }
func (m *mockToken) WaitTimeout(time.Duration) bool { return true }
func (m *mockToken) Done() <-chan struct{}          { return m.ch }
func (m *mockToken) Error() error                   { return m.err }

// newTestOptions creates a new OptionsCtx for testing
func newTestOptions(ctx context.Context, cancel context.CancelFunc) *OptionsCtx {
	return &OptionsCtx{
		Context:       ctx,
		CancelFunc:    cancel,
		Servers:       []string{"tcp://localhost:1883"},
		User:          "test",
		Password:      "test",
		ClientPrefix:  "test-client",
		ClientNum:     1,
		ConnRate:      10,
		AutoReconnect: true,
		CleanSession:  true,
	}
}

func TestConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	options := newTestOptions(ctx, cancel)

	t.Run("Test Connection Manager Creation", func(t *testing.T) {
		manager := NewConnectionManager(options, 1)
		assert.NotNil(t, manager)
		assert.Equal(t, options, manager.optionsCtx)
		assert.Equal(t, 1, manager.keepTime)
	})

	t.Run("Test Connection Manager Operations", func(t *testing.T) {
		manager := NewConnectionManager(options, 1)

		// Add a mock client directly
		mockClient := newMockMQTTClient()
		manager.activeClients = append(manager.activeClients, mockClient)

		// Test keep connections
		err := manager.KeepConnections()
		assert.NoError(t, err)

		// Test disconnect all
		err = manager.DisconnectAll()
		assert.NoError(t, err)
	})

	t.Run("Test Connection Rate Limiter", func(t *testing.T) {
		limiter := rate.NewLimiter(rate.Limit(10), 1) // 10 connections per second
		assert.NotNil(t, limiter)

		// Test rate limiting
		start := time.Now()
		err := limiter.Wait(context.Background())
		assert.NoError(t, err)
		duration := time.Since(start)

		// First wait should be very quick
		assert.Less(t, duration, 100*time.Millisecond)
	})
}

func TestConnectionHelpers(t *testing.T) {
	t.Run("Test Connection Result", func(t *testing.T) {
		result := &ConnectionResult{
			ClientID: "test-client",
			Broker:   "tcp://localhost:1883",
			Success:  true,
			Error:    nil,
		}

		assert.Equal(t, "test-client", result.ClientID)
		assert.Equal(t, "tcp://localhost:1883", result.Broker)
		assert.True(t, result.Success)
		assert.NoError(t, result.Error)
	})
}
