package mqtt

import (
	"context"
	"testing"
	"time"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

// mockPublishToken implements mqtt.Token interface for testing publish operations
type mockPublishToken struct {
	err error
	waitTimeout bool
}

func newMockPublishToken(err error, waitTimeout bool) *mockPublishToken {
	return &mockPublishToken{
		err: err,
		waitTimeout: waitTimeout,
	}
}

func (m *mockPublishToken) Wait() bool { return true }
func (m *mockPublishToken) WaitTimeout(d time.Duration) bool { return !m.waitTimeout }
func (m *mockPublishToken) Done() <-chan struct{} { return make(chan struct{}) }
func (m *mockPublishToken) Error() error { return m.err }

// mockPublishClient implements mqtt.Client interface for testing publish operations
type mockPublishClient struct {
	connected bool
	publishToken mqtt.Token
}

func newMockPublishClient(token mqtt.Token) *mockPublishClient {
	return &mockPublishClient{
		connected: true,
		publishToken: token,
	}
}

func (m *mockPublishClient) Connect() mqtt.Token { return nil }
func (m *mockPublishClient) Disconnect(uint) {}
func (m *mockPublishClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	return m.publishToken
}
func (m *mockPublishClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token { return nil }
func (m *mockPublishClient) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token { return nil }
func (m *mockPublishClient) Unsubscribe(topics ...string) mqtt.Token { return nil }
func (m *mockPublishClient) AddRoute(topic string, callback mqtt.MessageHandler) {}
func (m *mockPublishClient) IsConnected() bool { return m.connected }
func (m *mockPublishClient) IsConnectionOpen() bool { return m.connected }
func (m *mockPublishClient) OptionsReader() mqtt.ClientOptionsReader { return mqtt.ClientOptionsReader{} }

func TestPublisher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	options := &OptionsCtx{
		Context:    ctx,
		CancelFunc: cancel,
		Servers:    []string{"tcp://localhost:1883"},
		User:       "test",
		Password:   "test",
	}

	t.Run("Test Publisher Creation", func(t *testing.T) {
		pub := NewPublisher(options, "test/topic", 1, 1, "test", 100, 0, 10, 1.0)
		assert.NotNil(t, pub)
		assert.Equal(t, 100, pub.payloadSize)
		assert.Equal(t, 0, pub.qos)
		assert.Equal(t, 10, pub.count)
		assert.Equal(t, 1.0, pub.rate)
		assert.Equal(t, 5*time.Second, pub.timeout)
		assert.False(t, pub.withTimestamp)
		assert.False(t, pub.retain)
	})

	t.Run("Test Publisher Settings", func(t *testing.T) {
		pub := NewPublisher(options, "test/topic", 1, 1, "test", 100, 0, 10, 1.0)
		
		// Test timeout setting
		newTimeout := 10 * time.Second
		pub.SetTimeout(newTimeout)
		assert.Equal(t, newTimeout, pub.timeout)

		// Test timestamp setting
		pub.SetWithTimestamp(true)
		assert.True(t, pub.withTimestamp)

		// Test retain setting
		pub.SetRetain(true)
		assert.True(t, pub.retain)

		// Test wait for clients setting
		pub.SetWaitForClients(true)
		assert.True(t, pub.waitForClients)
	})

	t.Run("Test Small Payload Size with Timestamp", func(t *testing.T) {
		pub := NewPublisher(options, "test/topic", 1, 1, "", 10, 0, 10, 1.0)
		pub.SetWithTimestamp(true)
		payload := pub.generateRandomPayload()
		parts := strings.Split(string(payload), "|")
		assert.Equal(t, 2, len(parts))
		timestamp, err := time.Parse(time.RFC3339Nano, parts[0])
		assert.NoError(t, err)
		assert.True(t, timestamp.Before(time.Now()) || timestamp.Equal(time.Now()))
	})

	t.Run("Test Topic Generator", func(t *testing.T) {
		// Test with single topic
		pub := NewPublisher(options, "test/topic", 1, 1, "", 100, 0, 10, 1.0)
		topic := pub.topicGenerator.NextTopic()
		assert.Equal(t, "test/topic", topic)

		// Test with multiple topics
		pub = NewPublisher(options, "test/topic/%d", 3, 1, "", 100, 0, 10, 1.0)
		topics := make(map[string]bool)
		for i := 0; i < 5; i++ {
			topic = pub.topicGenerator.NextTopic()
			topics[topic] = true
		}
		assert.Equal(t, 3, len(topics)) // Should cycle through 3 topics
	})

	t.Run("Test Publish Functionality", func(t *testing.T) {
		// Test successful publish with QoS 0
		pub := NewPublisher(options, "test/topic", 1, 1, "test-payload", 100, 0, 10, 1.0)
		successToken := newMockPublishToken(nil, false)
		client := newMockPublishClient(successToken)
		err := pub.publish(client, pub.topicGenerator)
		assert.NoError(t, err)

		// Test successful publish with QoS 1
		pub.qos = 1
		err = pub.publish(client, pub.topicGenerator)
		assert.NoError(t, err)

		// Test publish timeout with QoS 1
		timeoutToken := newMockPublishToken(nil, true)
		timeoutClient := newMockPublishClient(timeoutToken)
		pub.timeout = 1 * time.Millisecond
		err = pub.publish(timeoutClient, pub.topicGenerator)
		assert.NoError(t, err) // Error is handled in goroutine
	})

	t.Run("Test Error Cases", func(t *testing.T) {
		// Test with nil options
		assert.Panics(t, func() {
			NewPublisher(nil, "test/topic", 1, 1, "", 100, 0, 10, 1.0)
		})

		// Test with invalid topic number
		assert.Panics(t, func() {
			NewPublisher(options, "test/topic/%d", 0, 1, "", 100, 0, 10, 1.0)
		})
	})
}

func TestTopicGenerator(t *testing.T) {
	t.Run("Test Topic Generation", func(t *testing.T) {
		topicGen := NewTopicGenerator("test/topic/%d", 2, 1)
		assert.NotNil(t, topicGen)

		// Test topic generation
		topic1 := topicGen.NextTopic()
		assert.Equal(t, "test/topic/0", topic1)
		
		topic2 := topicGen.NextTopic()
		assert.Equal(t, "test/topic/1", topic2)

		// Test cycling back to first topic
		topic3 := topicGen.NextTopic()
		assert.Equal(t, "test/topic/0", topic3)

		// Test get current topic without advancing
		currentTopic := topicGen.GetTopic()
		assert.Equal(t, "test/topic/0", currentTopic)

		// Test get all topics
		allTopics := topicGen.GetTopics()
		assert.Equal(t, 2, len(allTopics))
		assert.Contains(t, allTopics, "test/topic/0")
		assert.Contains(t, allTopics, "test/topic/1")
	})
}
