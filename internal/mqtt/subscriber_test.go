package mqtt

import (
	"context"
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

func TestNewSubscriber(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// Test valid creation
	sub := NewSubscriber(options, "test/topic", 1, 0, 0)
	assert.NotNil(t, sub, "Subscriber should not be nil")
	assert.Equal(t, 0, sub.qos, "QoS should match")
	assert.Equal(t, "test/topic", sub.topicGenerator.TopicTemplate, "Topic should match")
	assert.Equal(t, 5*time.Second, sub.timeout, "Default timeout should be 5 seconds")
	assert.False(t, sub.parseTimestamp, "ParseTimestamp should be false by default")

	// Test with different QoS levels
	sub = NewSubscriber(options, "test/topic", 1, 0, 1)
	assert.Equal(t, 1, sub.qos, "QoS should be 1")

	sub = NewSubscriber(options, "test/topic", 1, 0, 2)
	assert.Equal(t, 2, sub.qos, "QoS should be 2")

	// Test with multiple topics
	sub = NewSubscriber(options, "test/topic", 3, 0, 0)
	topics := sub.topicGenerator.GetTopics()
	assert.Equal(t, 3, len(topics), "Should generate 3 topics")

	// Test timeout setting
	timeout := 5 * time.Second
	sub.SetTimeout(timeout)
	assert.Equal(t, timeout, sub.timeout, "Timeout should match")

	// Test parse timestamp setting
	sub.SetParseTimestamp(true)
	assert.True(t, sub.parseTimestamp, "ParseTimestamp should be true")
}

func TestSubscriberMessageHandling(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	sub := NewSubscriber(options, "test/topic", 1, 0, 0)
	sub.SetParseTimestamp(true)

	// Create mock client with message handler
	mockClient := &mockMQTTClient{
		opts: mqtt.NewClientOptions(),
	}
	mockClient.Connect()

	// Test message handling
	sub.handleClientConnect(mockClient, 0)

	// Get the message handler
	handler := mockClient.opts.DefaultPublishHandler

	// Verify message count is 0 initially
	assert.Equal(t, int64(0), sub.msgCount, "Initial message count should be 0")

	// Test cases for different message scenarios
	testCases := []struct {
		name           string
		message        *mockMessage
		expectedCount  int64
		expectLatency bool
	}{
		{
			name: "Basic message without timestamp",
			message: &mockMessage{
				topic:   "test/topic",
				payload: []byte("test message"),
				qos:     0,
			},
			expectedCount:  1,
			expectLatency: false,
		},
		{
			name: "Message with valid timestamp",
			message: &mockMessage{
				topic:   "test/topic",
				payload: []byte(time.Now().Format(time.RFC3339Nano) + "|test message"),
				qos:     1,
			},
			expectedCount:  2,
			expectLatency: true,
		},
		{
			name: "Message with invalid timestamp format",
			message: &mockMessage{
				topic:   "test/topic",
				payload: []byte("invalid_timestamp|test message"),
				qos:     1,
			},
			expectedCount:  3,
			expectLatency: false,
		},
		{
			name: "Empty message",
			message: &mockMessage{
				topic:   "test/topic",
				payload: []byte{},
				qos:     0,
			},
			expectedCount:  4,
			expectLatency: false,
		},
		{
			name: "Message with QoS 2",
			message: &mockMessage{
				topic:   "test/topic",
				payload: []byte("test message"),
				qos:     2,
			},
			expectedCount:  5,
			expectLatency: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler(mockClient, tc.message)
			assert.Equal(t, tc.expectedCount, sub.msgCount, "Message count should match for case: %s", tc.name)
		})
	}
}

func TestSubscriberRunSubscribe(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	sub := NewSubscriber(options, "test/topic", 1, 0, 0)

	// Test successful subscription
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel() // Simulate test completion
	}()
	err := sub.RunSubscribe()
	assert.NoError(t, err, "RunSubscribe should succeed")

	// Test with connection error
	options, cancel = setupTest()
	defer cancel()
	options.newClientFunc = mockNewClientFuncWithError(errors.New("connection failed"))
	sub = NewSubscriber(options, "test/topic", 1, 0, 0)
	err = sub.RunSubscribe()
	assert.Error(t, err, "RunSubscribe should fail on connection error")

}

func TestSubscriberMultipleTopics(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	sub := NewSubscriber(options, "test/topic", 3, 0, 0)
	mockClient := &mockMQTTClient{
		opts: mqtt.NewClientOptions(),
	}
	mockClient.Connect()

	// Test subscribing to multiple topics
	sub.handleClientConnect(mockClient, 0)

	// Get the message handler
	handler := mockClient.opts.DefaultPublishHandler

	// Test receiving messages on different topics
	topics := sub.topicGenerator.GetTopics()
	for i, topic := range topics {
		msg := &mockMessage{
			topic:   topic,
			payload: []byte("test message"),
			qos:     byte(i % 3), // Test different QoS levels
		}
		handler(mockClient, msg)
		assert.Equal(t, int64(i+1), sub.msgCount, "Message count should increment for each topic")
	}
}

func TestSubscriberContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	options := &OptionsCtx{
		Context:    ctx,
		CancelFunc: cancel,
		ClientNum:  1,
		Servers:    []string{"tcp://localhost:1883"},
	}

	sub := NewSubscriber(options, "test/topic", 1, 0, 0)
	mockClient := &mockMQTTClient{
		opts: mqtt.NewClientOptions(),
	}

	// Start the report goroutine
	sub.report()

	// Cancel the context after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// The report goroutine should exit when context is cancelled
	sub.handleClientConnect(mockClient, 0)
}

