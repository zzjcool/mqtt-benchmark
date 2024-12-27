package mqtt

import (
	"context"
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

// mockPublishToken implements mqtt.Token interface for testing publish operations
type mockPublishToken struct {
	err      error
	waitTime time.Duration
	topic    string
	payload  interface{}
	qos      byte
	retain   bool
	done     chan struct{}
}

func newMockPublishToken(err error, waitTime time.Duration, topic string, payload interface{}, qos byte, retain bool) *mockPublishToken {
	t := &mockPublishToken{
		err:      err,
		waitTime: waitTime,
		topic:    topic,
		payload:  payload,
		qos:      qos,
		retain:   retain,
		done:     make(chan struct{}),
	}
	go func() {
		if t.waitTime > 0 {
			time.Sleep(t.waitTime)
		}
		close(t.done)
	}()
	return t
}

func (t *mockPublishToken) Wait() bool {
	if t.waitTime > 0 {
		time.Sleep(t.waitTime)
	}
	return t.err == nil
}

func (t *mockPublishToken) WaitTimeout(d time.Duration) bool {
	if t.waitTime > d {
		return false
	}
	if t.waitTime > 0 {
		time.Sleep(t.waitTime)
	}
	return true
}

func (t *mockPublishToken) Error() error { return t.err }
func (t *mockPublishToken) Done() <-chan struct{} { return t.done }

// mockMQTTClientWithPublish implements mqtt.Client interface for testing publish operations
type mockMQTTClientWithPublish struct {
	connected     bool
	opts         *mqtt.ClientOptions
	publishToken *mockPublishToken
	publishDelay time.Duration
	publishError error
	publishCalled bool
}

func (m *mockMQTTClientWithPublish) IsConnected() bool { return m.connected }
func (m *mockMQTTClientWithPublish) IsConnectionOpen() bool { return m.connected }
func (m *mockMQTTClientWithPublish) Connect() mqtt.Token { return newMockPublishToken(nil, 0, "", nil, 0, false) }
func (m *mockMQTTClientWithPublish) Disconnect(quiesce uint) {}
func (m *mockMQTTClientWithPublish) Publish(topic string, qos byte, retain bool, payload interface{}) mqtt.Token {
	m.publishCalled = true
	m.publishToken = newMockPublishToken(m.publishError, m.publishDelay, topic, payload, qos, retain)
	return m.publishToken
}
func (m *mockMQTTClientWithPublish) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	return newMockPublishToken(nil, 0, "", nil, 0, false)
}
func (m *mockMQTTClientWithPublish) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token {
	return newMockPublishToken(nil, 0, "", nil, 0, false)
}
func (m *mockMQTTClientWithPublish) Unsubscribe(topics ...string) mqtt.Token {
	return newMockPublishToken(nil, 0, "", nil, 0, false)
}
func (m *mockMQTTClientWithPublish) AddRoute(topic string, callback mqtt.MessageHandler) {}
func (m *mockMQTTClientWithPublish) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.NewOptionsReader(m.opts)
}

// setupTestPublisher creates a test publisher with mock client
func setupTestPublisher(topic string, payload string, payloadSize int, qos int, count int, rate float64) (*Publisher, *mockMQTTClientWithPublish) {
	ctx, cancel := context.WithCancel(context.Background())
	
	mockClient := &mockMQTTClientWithPublish{
		connected: true,
		opts: mqtt.NewClientOptions(),
	}

	options := &OptionsCtx{
		Context:     ctx,
		CancelFunc:  cancel,
		Servers:     []string{"tcp://localhost:1883"},
		ClientNum:   1,
		ClientIndex: 0,
		WriteTimeout: 5,
	}

	options.OnConnect = func(client mqtt.Client, idx uint32) {
		// Do nothing in test
	}

	publisher := NewPublisher(options, topic, 1, 0, payload, payloadSize, qos, count, rate)
	return publisher, mockClient
}

func TestPublisher_BasicPublish(t *testing.T) {
	publisher, mockClient := setupTestPublisher("test/topic", "test-payload", 0, 1, 1, 1.0)
	
	// Test basic publish
	errChan := publisher.asyncPublish(mockClient, publisher.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.NoError(t, err)
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for publish")
	}
	
	assert.True(t, mockClient.publishCalled)
	assert.Equal(t, "test/topic", mockClient.publishToken.topic)
	assert.Equal(t, []byte("test-payload"), mockClient.publishToken.payload)
	assert.Equal(t, byte(1), mockClient.publishToken.qos)
	assert.False(t, mockClient.publishToken.retain)
}

func TestPublisher_PublishWithTimestamp(t *testing.T) {
	publisher, mockClient := setupTestPublisher("test/topic", "", 100, 1, 1, 1.0)
	publisher.SetWithTimestamp(true)
	
	// Test publish with timestamp
	errChan := publisher.asyncPublish(mockClient, publisher.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.NoError(t, err)
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for publish")
	}
	
	assert.True(t, mockClient.publishCalled)
	
	// Verify payload contains timestamp
	payload, ok := mockClient.publishToken.payload.([]byte)
	assert.True(t, ok)
	assert.Contains(t, string(payload), "T") // Simple check for ISO timestamp format
}

func TestPublisher_PublishWithRetain(t *testing.T) {
	publisher, mockClient := setupTestPublisher("test/topic", "test-payload", 0, 1, 1, 1.0)
	publisher.SetRetain(true)
	
	// Test publish with retain flag
	errChan := publisher.asyncPublish(mockClient, publisher.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.NoError(t, err)
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for publish")
	}
	
	assert.True(t, mockClient.publishCalled)
	assert.True(t, mockClient.publishToken.retain)
}

func TestPublisher_PublishError(t *testing.T) {
	// Setup publisher with mock client
	pub, mockClient := setupTestPublisher("test/topic", "", 100, 1, 1, 1.0)
	mockClient.publishError = errors.New("publish error")

	// Test publish with error
	errChan := pub.asyncPublish(mockClient, pub.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.Error(t, err)
		assert.Equal(t, "publish error", err.Error())
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for error")
	}

	// Verify channel is closed
	_, ok := <-errChan
	assert.False(t, ok, "channel should be closed")
}

func TestPublisher_PublishQoS0(t *testing.T) {
	// Setup publisher with QoS 0
	pub, mockClient := setupTestPublisher("test/topic", "", 100, 0, 1, 1.0)

	// Test publish with QoS 0
	errChan := pub.asyncPublish(mockClient, pub.topicGenerator)
	assert.Nil(t, errChan, "QoS 0 should return nil channel")
	assert.True(t, mockClient.publishCalled)
}

func TestPublisher_PublishTimeout(t *testing.T) {
	// Setup publisher with mock client that will timeout
	pub, mockClient := setupTestPublisher("test/topic", "", 100, 1, 1, 1.0)
	mockClient.publishDelay = 2 * time.Second
	pub.SetTimeout(100 * time.Millisecond)

	// Test publish with timeout
	errChan := pub.asyncPublish(mockClient, pub.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for error")
	}

	// Verify channel is closed
	_, ok := <-errChan
	assert.False(t, ok, "channel should be closed")
}

func TestPublisher_PublishSuccess(t *testing.T) {
	// Setup publisher with mock client
	pub, mockClient := setupTestPublisher("test/topic", "", 100, 1, 1, 1.0)

	// Test successful publish
	errChan := pub.asyncPublish(mockClient, pub.topicGenerator)
	assert.NotNil(t, errChan)

	select {
	case err, ok := <-errChan:
		assert.True(t, ok, "channel should be open")
		assert.NoError(t, err)
	case <-time.After(time.Second):
		assert.Fail(t, "timeout waiting for publish")
	}

	// Verify channel is closed
	_, ok := <-errChan
	assert.False(t, ok, "channel should be closed")
	assert.True(t, mockClient.publishCalled)
}

func TestPublisher_PublishWithInflight(t *testing.T) {
	// Create a publisher with inflight limit of 2
	publisher, mockClient := setupTestPublisher("test", "", 100, 1, 5, 100.0)
	publisher.SetInflight(2)
	
	// Mock client behavior
	mockClient.connected = true
	mockClient.publishDelay = 100 * time.Millisecond // Add delay to simulate network latency
	
	// Directly test the asyncPublish function
	errCh := publisher.asyncPublish(mockClient, publisher.topicGenerator)
	
	// Wait for publish to complete
	err := <-errCh
	assert.NoError(t, err)
	assert.True(t, mockClient.publishCalled)
}
