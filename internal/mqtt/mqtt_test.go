package mqtt

import (
	"context"
	"errors"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
)

// mockToken implements mqtt.Token interface for testing
type mockToken struct {
	err      error
	waitTime time.Duration
}

func (t *mockToken) Wait() bool { return true }
func (t *mockToken) WaitTimeout(d time.Duration) bool {
	if t.waitTime > d {
		return false
	}
	time.Sleep(t.waitTime)
	return true
}
func (t *mockToken) Error() error          { return t.err }
func (t *mockToken) Done() <-chan struct{} { return nil }

// mockPublishToken implements mqtt.Token interface for testing publish operations
type mockPublishToken struct {
	mockToken
	topic    string
	payload  []byte
	retained bool
	qos      byte
}

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
	if m.opts.OnConnect != nil {
		go m.opts.OnConnect(m)
	}
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

func (m *mockMQTTClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return &mockPublishToken{
			mockToken: mockToken{err: errors.New("client not connected")},
		}
	}

	// Convert payload to []byte
	var payloadBytes []byte
	switch p := payload.(type) {
	case []byte:
		payloadBytes = p
	case string:
		payloadBytes = []byte(p)
	default:
		return &mockPublishToken{
			mockToken: mockToken{err: errors.New("invalid payload type")},
		}
	}

	return &mockPublishToken{
		mockToken: mockToken{},
		topic:     topic,
		payload:   payloadBytes,
		retained:  retained,
		qos:       qos,
	}
}

func (m *mockMQTTClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return &mockToken{err: errors.New("client not connected")}
	}

	// Store the callback in the options for testing
	m.opts.SetDefaultPublishHandler(callback)

	return &mockToken{}
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
func setupTest() (*OptionsCtx, context.CancelFunc) {
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
	}, []string{"broker"})
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
	metrics.MQTTMessagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_messages_received",
		Help: "Number of MQTT messages received",
	})
	metrics.MQTTMessageReceiveRate = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_message_receive_rate",
		Help: "Rate of MQTT messages received",
	})
	metrics.MQTTMessageQosDistribution = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_message_qos_distribution",
		Help: "Distribution of MQTT message QoS levels",
	}, []string{"qos"})
	metrics.MQTTMessagePayloadSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "mqtt_message_payload_size",
		Help: "Size of MQTT message payloads",
	})
	metrics.MQTTMessageReceiveLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "mqtt_message_receive_latency",
		Help: "Latency of MQTT message reception",
	})
	metrics.MQTTSubscriptionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_subscription_errors",
		Help: "Number of MQTT subscription errors",
	}, []string{"topic", "type"})

	reg.MustRegister(
		metrics.MQTTConnectionRateLimit,
		metrics.MQTTConnections,
		metrics.MQTTConnectionAttempts,
		metrics.MQTTConnectionErrors,
		metrics.MQTTConnectionTime,
		metrics.MQTTNewConnections,
		metrics.MQTTMessagesReceived,
		metrics.MQTTMessageReceiveRate,
		metrics.MQTTMessageQosDistribution,
		metrics.MQTTMessagePayloadSize,
		metrics.MQTTMessageReceiveLatency,
		metrics.MQTTSubscriptionErrors,
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
		newClientFunc:    mockNewClientFunc,
	}

	return options, cancel
}


// mockMessage implements mqtt.Message interface for testing
type mockMessage struct {
	duplicate bool
	qos       byte
	retained  bool
	topic     string
	messageID uint16
	payload   []byte
}

func (m *mockMessage) Duplicate() bool     { return m.duplicate }
func (m *mockMessage) Qos() byte          { return m.qos }
func (m *mockMessage) Retained() bool      { return m.retained }
func (m *mockMessage) Topic() string       { return m.topic }
func (m *mockMessage) MessageID() uint16   { return m.messageID }
func (m *mockMessage) Payload() []byte     { return m.payload }
func (m *mockMessage) Ack()               {}
