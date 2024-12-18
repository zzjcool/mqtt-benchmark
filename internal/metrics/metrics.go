package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Connection metrics
	MQTTConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_active_connections",
		Help: "The current number of active MQTT connections",
	}, []string{"broker"})

	MQTTConnectionAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_benchmark_connection_attempts_total",
		Help: "The total number of MQTT connection attempts",
	}, []string{"broker", "result"}) // result can be "success" or "failure"

	MQTTConnectionErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_benchmark_connection_errors_total",
		Help: "The total number of MQTT connection errors",
	}, []string{"broker", "error_type"}) // error_type can be "timeout", "auth_failed", "network", etc.

	// Connection timing metrics
	MQTTConnectionTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mqtt_benchmark_connection_time_seconds",
		Help:    "Time taken to establish MQTT connections",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // from 1ms to ~1s
	}, []string{"broker"})

	// Connection rate metrics
	MQTTConnectionRateLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_connection_rate_limit",
		Help: "The configured connection rate limit (connections per second)",
	})

	MQTTNewConnections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mqtt_benchmark_new_connections_total",
		Help: "The total number of new MQTT connections established",
	}, []string{"broker"})

	// Connection pool metrics
	MQTTConnectionPoolSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_connection_pool_size",
		Help: "The configured size of the connection pool",
	})

	MQTTConnectionPoolActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_connection_pool_active",
		Help: "The number of currently active connections in the pool",
	})

	MQTTConnectionPoolWaiting = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_connection_pool_waiting",
		Help: "The number of goroutines waiting for a connection",
	})

	// Message metrics
	MQTTMessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_benchmark_messages_sent_total",
		Help: "The total number of MQTT messages sent",
	})

	MQTTMessagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_benchmark_messages_received_total",
		Help: "The total number of MQTT messages received",
	})

	MQTTMessageLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "mqtt_benchmark_message_latency_seconds",
		Help:    "The latency of MQTT messages",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // from 1ms to ~1s
	})

	// Publish metrics
	MQTTPublishTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_benchmark_publish_total",
		Help: "The total number of publish attempts",
	})

	MQTTPublishSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_benchmark_publish_success_total",
		Help: "The total number of successful publishes",
	})

	MQTTPublishFailureTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mqtt_benchmark_publish_failure_total",
		Help: "The total number of failed publishes",
	})

	MQTTPublishRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_publish_rate",
		Help: "The configured publish rate (messages per second)",
	})

	MQTTPublishActualRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mqtt_benchmark_publish_actual_rate",
		Help: "The actual publish rate (messages per second)",
	})

	MQTTPublishLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "mqtt_benchmark_publish_latency_seconds",
		Help:    "Time taken to publish messages",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // from 1ms to ~1s
	})
)
