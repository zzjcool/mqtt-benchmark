package mqtt

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
)

func TestPublisherAndSubscriberReport(t *testing.T) {
	// Setup
	ctx, cancel := context.WithCancel(context.Background())
	options := &OptionsCtx{
		Context:    ctx,
		ClientNum:  2,
		Servers:    []string{"tcp://localhost:1883"},
	}
	logger := zap.NewNop()

	// Test Publisher Report
	t.Run("Publisher Report", func(t *testing.T) {
		publisher := NewPublisher(options, "test/topic", 1, 0, "", 100, 1, 10, 1.0)
		publisher.log = logger

		// Reset metrics by creating new ones
		metrics.MQTTPublishTotal.Add(0)
		metrics.MQTTPublishSuccessTotal.Add(0)
		metrics.MQTTPublishFailureTotal.Add(0)
		metrics.MQTTPublishRate.Set(0)
		metrics.MQTTPublishActualRate.Set(0)
		metrics.MQTTConnections.WithLabelValues(options.Servers...).Set(0)

		// Start report
		publisher.report()

		// Simulate some metrics changes
		metrics.MQTTConnections.WithLabelValues(options.Servers...).Set(2)
		metrics.MQTTPublishSuccessTotal.Add(100)
		metrics.MQTTPublishLatency.Observe(0.1)

		// Wait for a few report cycles
		time.Sleep(2 * time.Second)

		// Check metrics
		assert.Equal(t, float64(2), metrics.GetGaugeVecValue(metrics.MQTTConnections, options.Servers...))
		assert.Equal(t, float64(100), metrics.GetCounterValue(metrics.MQTTPublishSuccessTotal))
	})

	// Test Subscriber Report
	t.Run("Subscriber Report", func(t *testing.T) {
		subscriber := NewSubscriber(options, "test/topic", 1, 0, 1)
		subscriber.log = logger

		// Reset metrics by creating new ones
		metrics.MQTTMessagesReceived.Add(0)

		// Start report
		subscriber.report()

		// Simulate some metrics changes
		metrics.MQTTMessagesReceived.Add(100)
		metrics.MQTTMessagePayloadSize.Observe(1000)
		metrics.MQTTMessageReceiveLatency.Observe(0.1)

		// Wait for a few report cycles
		time.Sleep(2 * time.Second)

		// Check metrics
		assert.Equal(t, float64(100), metrics.GetCounterValue(metrics.MQTTMessagesReceived))
	})

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond) // Wait for goroutine to exit
}
