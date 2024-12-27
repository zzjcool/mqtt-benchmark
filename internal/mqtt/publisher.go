package mqtt

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Publisher handles MQTT message publishing operations
type Publisher struct {
	optionsCtx     *OptionsCtx
	topicGenerator *TopicGenerator
	payload        string
	payloadSize    int
	qos            int
	count          int
	rate           float64 // Messages per second per client
	timeout        time.Duration
	log            *zap.Logger
	withTimestamp  bool // Add timestamp to payload
	retain         bool
	waitForClients bool
	inflight       int  // Maximum inflight messages for QoS 1 and 2
}

// NewPublisher creates a new Publisher
func NewPublisher(options *OptionsCtx, topic string, topicNum int, clientIndex uint32, payload string, payloadSize int, qos int, count int, rate float64) *Publisher {
	if options == nil {
		panic("options cannot be nil")
	}
	if topicNum <= 0 {
		panic("topicNum must be greater than 0")
	}

	topicGenerator := NewTopicGenerator(topic, topicNum, clientIndex)

	return &Publisher{
		optionsCtx:     options,
		topicGenerator: topicGenerator,
		payload:        payload,
		payloadSize:    payloadSize,
		qos:            qos,
		count:          count,
		rate:           rate,
		timeout:        5 * time.Second,
		log:            logger.GetLogger(),
		withTimestamp:  false,
		retain:         false,
		inflight:       1,
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

// SetWaitForClients sets whether to wait for other clients to be ready
func (p *Publisher) SetWaitForClients(waitForClients bool) {
	p.waitForClients = waitForClients
}

// SetInflight sets the maximum number of inflight messages
func (p *Publisher) SetInflight(inflight int) {
	p.inflight = inflight
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

// asyncPublish attempts to asyncPublish a message
func (p *Publisher) asyncPublish(client mqtt.Client, topicGen *TopicGenerator) chan error {

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

	result := make(chan error, 1)
	// For QoS 1 and 2, handle asynchronously
	go func() {
		defer close(result)
		if token.WaitTimeout(p.timeout) {
			err := token.Error()
			if err == nil {
				metrics.MQTTPublishSuccessTotal.Inc()
				latency := time.Since(startTime)
				metrics.MQTTPublishLatency.Observe(latency.Seconds())
				result <- nil
			} else {
				metrics.MQTTPublishFailureTotal.Inc()
				result <- err
			}
		} else {
			metrics.MQTTPublishFailureTotal.Inc()
			timeoutErr := errors.New("publish operation timeout")
			p.log.Warn(timeoutErr.Error(),
				zap.String("topic", topic),
				zap.Error(token.Error()),
				zap.Duration("elapsed_time", time.Since(startTime)))
			result <- timeoutErr
		}
	}()

	return result
}

// RunPublish starts the publishing process
func (p *Publisher) RunPublish() error {
	p.log.Info("Starting publish test",
		zap.String("topic", p.topicGenerator.NextTopic()),
		zap.Int("qos", p.qos),
		zap.Int("count", p.count),
		zap.Float64("rate", p.rate),
		zap.Int64("timeout_seconds", int64(p.timeout/time.Second)))

	// Create connection manager with auto reconnect enabled
	p.optionsCtx.ConnectRetryInterval = 5 // 5 seconds retry interval

	p.optionsCtx.OnConnect = func(client mqtt.Client, idx uint32) {
		clientOptionsReader := client.OptionsReader()
		clientID := clientOptionsReader.ClientID()
		// Create a new TopicGenerator for each client with its own clientID
		clientTopicGen := NewTopicGenerator(p.topicGenerator.TopicTemplate, p.topicGenerator.TopicNum, idx)
		p.log.Debug("Publisher goroutine started",
			zap.Uint32("client_id", idx))
		limiter := rate.NewLimiter(rate.Limit(p.rate), 1)

		inflightCh := make(chan struct{}, p.inflight)
		if p.inflight > 0 {
			// Initialize inflightCh with tokens
			for i := 0; i < p.inflight; i++ {
				inflightCh <- struct{}{}
			}
		}

		for {
			select {
			case <-p.optionsCtx.Done():
				p.log.Debug("Publisher goroutine cancelled", zap.String("client_id", clientID))
				return
			default:
				// Wait for rate limiter
				if err := limiter.Wait(p.optionsCtx); err != nil {
					if errors.Is(err, p.optionsCtx.Err()) {
						p.log.Debug("Publisher goroutine cancelled",
							zap.String("client_id", clientID))
						return
					}
					p.log.Error("Rate limiter error",
						zap.Error(err),
						zap.String("client_id", clientID))
					return
				}

				// Check inflight limit if set
				if p.inflight > 0 {
					select {
					case <-inflightCh:
						// Got a token, proceed with publish
					case <-p.optionsCtx.Done():
						return
					}
				}

				// Publish message
				errCh := p.asyncPublish(client, clientTopicGen)

				// Start a goroutine to handle the publish result
				if p.inflight > 0 {
					go func() {
						<-errCh // Wait for publish to complete
						inflightCh <- struct{}{} // Return the token
					}()
				}
			}
		}
	}

	p.report()

	connManager := NewConnectionManager(p.optionsCtx, 0)
	if err := connManager.RunConnections(); err != nil {
		return err
	}

	// Get active clients
	clients := connManager.activeClients
	defer connManager.DisconnectAll()

	if errors.Is(p.optionsCtx.Err(), context.Canceled) {
		return nil
	}
	if len(clients) == 0 {
		p.log.Error("No active clients available")
		return nil
	}

	<-p.optionsCtx.Done()

	p.log.Info("Publish test completed")

	// Disconnect all clients

	return nil
}

func (p *Publisher) report() {
	// Initialize metrics
	metrics.MQTTPublishTotal.Add(0)
	metrics.MQTTPublishSuccessTotal.Add(0)
	metrics.MQTTPublishFailureTotal.Add(0)
	// Calculate target rate as messages per second for all clients
	targetRate := float64(p.optionsCtx.ClientNum) * p.rate
	metrics.MQTTPublishRate.Set(targetRate)

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		prePublishSuccessTotal := uint64(0)
		prePublishLatencyTotal := float64(metrics.GetHistogramValue(metrics.MQTTPublishLatency))
		for {
			select {
			case <-p.optionsCtx.Done():
				return
			case <-ticker.C:
				connectedCount := uint32(metrics.GetGaugeVecValue(metrics.MQTTConnections, p.optionsCtx.Servers...))
				publishSuccessTotal := uint64(metrics.GetCounterValue(metrics.MQTTPublishSuccessTotal))
				publishLatencyTotal := float64(metrics.GetHistogramValue(metrics.MQTTPublishLatency))
				rate := publishSuccessTotal - prePublishSuccessTotal
				metrics.MQTTPublishActualRate.Set(float64(rate))

				if connectedCount == 0 || rate == 0 {
					continue
				}
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

}
