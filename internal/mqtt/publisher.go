package mqtt

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Publisher handles MQTT message publishing operations
type Publisher struct {
	options        *Options
	topicGenerator *TopicGenerator
	payload        string
	payloadSize    int
	qos            int
	count          int
	interval       int
	timeout        time.Duration
	log            *zap.Logger
	withTimestamp  bool // Add timestamp to payload
	retain         bool
}

// NewPublisher creates a new Publisher
func NewPublisher(options *Options, topic string, topicNum int, clientIndex int, payload string, payloadSize int, qos int, count int, interval int) *Publisher {
	topicGenerator := NewTopicGenerator(topic, topicNum, clientIndex)

	return &Publisher{
		options:        options,
		topicGenerator: topicGenerator,
		payload:        payload,
		payloadSize:    payloadSize,
		qos:            qos,
		count:          count,
		interval:       interval,
		timeout:        5 * time.Second,
		log:            logger.GetLogger(),
		withTimestamp:  false,
		retain:         false,
	}
}

// SetTimeout sets the publish timeout duration
func (p *Publisher) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}

// SetWithTimestamp sets whether to add timestamp to payload
func (p *Publisher) SetWithTimestamp(withTimestamp bool) {
	p.withTimestamp = withTimestamp
}

// SetRetain sets whether to retain the message
func (p *Publisher) SetRetain(retain bool) {
	p.retain = retain
}

// generateRandomPayload generates a random payload of specified size
func (p *Publisher) generateRandomPayload() []byte {
	if p.withTimestamp {
		// Calculate timestamp size including separator
		timestamp := time.Now().Format(time.RFC3339Nano)
		timestampSize := len(timestamp) + 1 // +1 for separator

		var payload []byte
		if p.payload != "" {
			payload = []byte(p.payload)
		} else {
			// Adjust payload size to account for timestamp
			adjustedSize := p.payloadSize - timestampSize
			if adjustedSize <= 0 {
				// If timestamp is larger than desired size, just return timestamp
				return append([]byte(timestamp), '|')
			}
			payload = make([]byte, adjustedSize)
			rand.Read(payload)
		}

		// Combine timestamp and payload
		finalPayload := make([]byte, 0, timestampSize+len(payload))
		finalPayload = append(finalPayload, []byte(timestamp)...)
		finalPayload = append(finalPayload, '|')
		finalPayload = append(finalPayload, payload...)
		return finalPayload
	}

	// No timestamp case
	if p.payload != "" {
		return []byte(p.payload)
	}
	payload := make([]byte, p.payloadSize)
	rand.Read(payload)
	return payload
}

// publishWithRetry attempts to publish a message with retry logic
func (p *Publisher) publishWithRetry(client mqtt.Client, topicGen *TopicGenerator, wg *sync.WaitGroup) error {

	// Generate payload
	payload := p.generateRandomPayload()
	topic := topicGen.NextTopic()
	p.log.Debug("Publishing message",
		zap.String("topic", topic),
		zap.Int("payload_size", len(payload)))

	startTime := time.Now()
	token := client.Publish(topic, byte(p.qos), p.retain, payload)
	metrics.MQTTPublishTotal.Inc()

	// For QoS 0, don't wait for confirmation
	if p.qos == 0 {
		metrics.MQTTPublishSuccessTotal.Inc()
		latency := time.Since(startTime)
		metrics.MQTTPublishLatency.Observe(latency.Seconds())
		return nil
	}

	// For QoS 1 and 2, handle asynchronously
	wg.Add(1)
	go func() {
		defer wg.Done()
		if token.WaitTimeout(p.timeout) {
			if token.Error() == nil {
				metrics.MQTTPublishSuccessTotal.Inc()
				latency := time.Since(startTime)
				metrics.MQTTPublishLatency.Observe(latency.Seconds())
				return
			}

		} else {
			p.log.Warn("Publish operation timed out",
				zap.String("topic", topic),
				zap.Duration("elapsed_time", time.Since(startTime)))
		}
		metrics.MQTTPublishFailureTotal.Inc()

	}()

	return nil
}

// RunPublish starts the publishing process
func (p *Publisher) RunPublish() error {
	p.log.Info("Starting publish test",
		zap.String("topic", p.topicGenerator.NextTopic()),
		zap.Int("qos", p.qos),
		zap.Int("count", p.count),
		zap.Int("interval", p.interval),
		zap.Int64("timeout_seconds", int64(p.timeout/time.Second)))

	// Create connection manager with auto reconnect enabled
	p.options.ConnectRetryInterval = 5 // 5 seconds retry interval
	p.options.ConnectTimeout = 5       // 5 seconds connect timeout

	connManager := NewConnectionManager(p.options, 0)
	if err := connManager.RunConnections(); err != nil {
		return err
	}

	// Get active clients
	clients := connManager.activeClients
	if len(clients) == 0 {
		p.log.Error("No active clients available")
		return nil
	}

	// Initialize metrics
	metrics.MQTTPublishTotal.Add(0)
	metrics.MQTTPublishSuccessTotal.Add(0)
	metrics.MQTTPublishFailureTotal.Add(0)
	// Calculate target rate as messages per second for all clients
	targetRate := float64(len(clients)) * (1000.0 / float64(p.interval))
	metrics.MQTTPublishRate.Set(targetRate)

	// Start a goroutine to track actual publish rate
	stopRateTracker := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		prePublishSuccessTotal := uint64(0)
		prePublishLatencyTotal := float64(metrics.GetHistogramValue(metrics.MQTTPublishLatency))
		for {
			select {
			case <-stopRateTracker:
				return
			case <-ticker.C:
				connectedCount := uint32(metrics.GetGaugeVecValue(metrics.MQTTConnections, p.options.Servers...))
				publishSuccessTotal := uint64(metrics.GetCounterValue(metrics.MQTTPublishSuccessTotal))
				publishLatencyTotal := float64(metrics.GetHistogramValue(metrics.MQTTPublishLatency))
				rate := publishSuccessTotal - prePublishSuccessTotal
				metrics.MQTTPublishActualRate.Set(float64(rate))
				avgLatency := 0.0
				if rate != 0 {
					avgLatency = (publishLatencyTotal - prePublishLatencyTotal) / float64(rate) * 1000
				}

				p.log.Info("Publishing at rate",
					zap.Uint64("rate", rate),
					zap.Uint64("publish_success_total", publishSuccessTotal),
					zap.Uint32("connected", connectedCount),
					zap.Uint16("publish_latency_ms", uint16(avgLatency)))
				prePublishSuccessTotal = publishSuccessTotal
				prePublishLatencyTotal = publishLatencyTotal
			}
		}
	}()

	// Create error channel to track fatal errors
	errChan := make(chan error, len(clients))
	var wg sync.WaitGroup
	ctx := context.Background()

	for i, client := range clients {
		wg.Add(1)
		// Create rate limiter for each client with burst = rate to allow catching up
		burstSize := int(1000.0 / float64(p.interval))
		if burstSize < 1 {
			burstSize = 1
		}
		clientLimiter := rate.NewLimiter(rate.Limit(burstSize), 1)
		go func(c mqtt.Client, clientID int, limiter *rate.Limiter) {
			defer wg.Done()
			// Create a new TopicGenerator for each client with its own clientID
			clientTopicGen := NewTopicGenerator(p.topicGenerator.TopicTemplate, p.topicGenerator.TopicNum, clientID)
			p.log.Debug("Publisher goroutine started",
				zap.Int("client_id", clientID))

			for msgNum := 0; p.count == 0 || msgNum < p.count/len(clients); msgNum++ {
				select {
				case <-ctx.Done():
					p.log.Debug("Publisher goroutine cancelled", zap.Int("client_id", clientID))
					return
				default:
					// Wait for rate limiter
					if err := limiter.Wait(ctx); err != nil {
						p.log.Error("Rate limiter error",
							zap.Error(err),
							zap.Int("client_id", clientID))
						errChan <- err
						return
					}

					p.publishWithRetry(c, clientTopicGen, &wg)

				}
			}
			p.log.Debug("Publisher goroutine completed",
				zap.Int("client_id", clientID),
				zap.Int("messages_published", p.count/len(clients)))
		}(client, i, clientLimiter)
	}

	// Wait for all publishers to complete
	wg.Wait()
	close(errChan)
	close(stopRateTracker)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			p.log.Error("Error during publishing", zap.Error(err))
		}
	}

	p.log.Info("Publish test completed")

	// Wait a short time to ensure all QoS 1/2 messages are acknowledged
	time.Sleep(100 * time.Millisecond)

	// Disconnect all clients
	connManager.DisconnectAll()
	return nil
}
