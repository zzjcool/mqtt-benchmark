package mqtt

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

func TestNewPublisher(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// Test valid creation
	pub := NewPublisher(options, "test/topic", 1, 0, "test", 10, 0, 100, 10.0)
	assert.NotNil(t, pub, "Publisher should not be nil")
	assert.Equal(t, "test", pub.payload, "Payload should match")
	assert.Equal(t, 10, pub.payloadSize, "Payload size should match")
	assert.Equal(t, 0, pub.qos, "QoS should match")
	assert.Equal(t, 100, pub.count, "Count should match")
	assert.Equal(t, 10.0, pub.rate, "Rate should match")


	pub.SetWithTimestamp(true)
	assert.True(t, pub.withTimestamp, "WithTimestamp should be true")

	pub.SetRetain(true)
	assert.True(t, pub.retain, "Retain should be true")

	pub.SetWaitForClients(true)
	assert.True(t, pub.waitForClients, "WaitForClients should be true")

	pub.SetInflight(10)
	assert.Equal(t, 10, pub.inflight, "Inflight should be updated")
}

func TestGenerateRandomPayload(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// Test with fixed payload
	pub := NewPublisher(options, "test/topic", 1, 0, "test", 10, 0, 100, 10.0)
	payload := pub.generateRandomPayload()
	assert.Equal(t, []byte("test"), payload, "Fixed payload should match")

	// Test with random payload
	pub = NewPublisher(options, "test/topic", 1, 0, "", 10, 0, 100, 10.0)
	payload = pub.generateRandomPayload()
	assert.Equal(t, 10, len(payload), "Random payload length should match")

	// Test with timestamp
	pub.SetWithTimestamp(true)
	payload = pub.generateRandomPayload()
	assert.Greater(t, len(payload), 20, "Payload with timestamp should be longer")
	assert.Contains(t, string(payload), "|", "Payload should contain timestamp separator")
}

func TestPublishMessage(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	pub := NewPublisher(options, "test/topic", 1, 0, "test", 10, 1, 100, 10.0)
	
	// Use mock client from mqtt_test.go
	mockClient := mockNewClientFunc(mqtt.NewClientOptions())
	mockClient.Connect()

	// Test successful publish
	errChan := pub.asyncPublish(mockClient, pub.topicGenerator)
	select {
	case err := <-errChan:
		assert.NoError(t, err, "Publish should succeed")
	case <-time.After(2 * time.Second):
		t.Fatal("Publish timeout")
	}

	// Test publish with disconnected client
	mockClient.Disconnect(0)
	errChan = pub.asyncPublish(mockClient, pub.topicGenerator)
	select {
	case err := <-errChan:
		assert.Error(t, err, "Publish should fail with disconnected client")
	case <-time.After(2 * time.Second):
		t.Fatal("Publish timeout")
	}
}

func TestRunPublish(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()
	options.ConnectRetry = false

	pub := NewPublisher(options, "test/topic", 1, 0, "test", 10, 0, 2, 10.0)
	pub.optionsCtx.WriteTimeout = 1
	pub.optionsCtx.newClientFunc = mockNewClientFunc

	// Test successful publish
	err := pub.RunPublish()
	assert.NoError(t, err, "RunPublish should succeed")

	// Test with connection error
	pub.optionsCtx.newClientFunc = mockNewClientFuncWithError(assert.AnError)
	err = pub.RunPublish()
	assert.Error(t, err, "RunPublish should fail with connection error")
}

func TestPublisherReport(t *testing.T) {
	options, cancel := setupTest()
	defer cancel()

	// Create a publisher with known configuration
	pub := NewPublisher(options, "test/topic", 1, 0, "test", 10, 0, 100, 10.0)
	pub.msgCount = 50 // Set a known message count

	// Call report method
	pub.report()

	// Since report only logs metrics, we can't directly assert its output
	// But we can verify it doesn't panic and completes successfully
}
