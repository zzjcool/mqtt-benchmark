package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestMetricsOperations(t *testing.T) {
	// Test connection metrics
	t.Run("Test MQTTConnections", func(t *testing.T) {
		broker := "test-broker"
		MQTTConnections.WithLabelValues(broker).Set(10)
		value := GetGaugeVecValue(MQTTConnections, broker)
		assert.Equal(t, float64(10), value)
	})

	t.Run("Test MQTTConnectionAttempts", func(t *testing.T) {
		broker := "test-broker"
		result := "success"
		MQTTConnectionAttempts.WithLabelValues(broker, result).Inc()
		value := GetCounterVecValue(MQTTConnectionAttempts, broker, result)
		assert.Equal(t, float64(1), value)
	})

	t.Run("Test MQTTConnectionErrors", func(t *testing.T) {
		broker := "test-broker"
		errorType := "timeout"
		MQTTConnectionErrors.WithLabelValues(broker, errorType).Add(2)
		value := GetCounterVecValue(MQTTConnectionErrors, broker, errorType)
		assert.Equal(t, float64(2), value)
	})

	t.Run("Test MQTTConnectionTime", func(t *testing.T) {
		MQTTConnectionTime.Observe(0.5)
		value := GetHistogramValue(MQTTConnectionTime)
		assert.Equal(t, float64(0.5), value) // Count should be 0.5 after one observation of 0.5
	})

	t.Run("Test Connection Pool Metrics", func(t *testing.T) {
		// Test pool size
		MQTTConnectionPoolSize.Set(100)
		poolSize := GetGaugeValue(MQTTConnectionPoolSize)
		assert.Equal(t, float64(100), poolSize)

		// Test active connections
		MQTTConnectionPoolActive.Set(50)
		activeConns := GetGaugeValue(MQTTConnectionPoolActive)
		assert.Equal(t, float64(50), activeConns)

		// Test waiting connections
		MQTTConnectionPoolWaiting.Set(10)
		waitingConns := GetGaugeValue(MQTTConnectionPoolWaiting)
		assert.Equal(t, float64(10), waitingConns)
	})
}

// Helper function for getting Gauge value
func GetGaugeValue(gauge prometheus.Gauge) float64 {
	var m prometheus.Metric
	var dtoMetric dto.Metric
	m, _ = gauge.(prometheus.Metric)
	m.Write(&dtoMetric)
	return *dtoMetric.Gauge.Value
}
