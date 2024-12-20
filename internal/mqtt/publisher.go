package mqtt

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Publisher handles MQTT message publishing operations
type Publisher struct {
	options     *Options
	topic       string
	payload     string
	payloadSize int
	qos         int
	count       int
	interval    int
	timeout     time.Duration
	log         *zap.Logger
	withTimestamp bool // Add timestamp to payload
}

// NewPublisher creates a new Publisher
func NewPublisher(options *Options, topic string, payload string, payloadSize int, qos int, count int, interval int) *Publisher {
	return &Publisher{
		options:     options,
		topic:       topic,
		payload:     payload,
		payloadSize: payloadSize,
		qos:         qos,
		count:       count,
		interval:    interval,
		timeout:     5 * time.Second,
		log:         logger.GetLogger(),
		withTimestamp: false,
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

// generateRandomPayload generates a random payload of specified size
func (p *Publisher) generateRandomPayload() []byte {
	var payload []byte
	if p.payload != "" {
		payload = []byte(p.payload)
	} else {
		payload = make([]byte, p.payloadSize)
		rand.Read(payload)
	}

	if p.withTimestamp {
		// Add timestamp to the beginning of payload
		timestamp := time.Now().Format(time.RFC3339Nano)
		timestampBytes := []byte(timestamp)
		// Add a separator between timestamp and payload
		finalPayload := make([]byte, 0, len(timestampBytes)+1+len(payload))
		finalPayload = append(finalPayload, timestampBytes...)
		finalPayload = append(finalPayload, '|') // Use | as separator
		finalPayload = append(finalPayload, payload...)
		return finalPayload
	}

	return payload
}

// publishWithRetry attempts to publish a message with retry logic
func (p *Publisher) publishWithRetry(client mqtt.Client, payload []byte, maxRetries int) error {
	if !client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	start := time.Now()
	token := client.Publish(p.topic, byte(p.qos), false, payload)
	metrics.MQTTPublishTotal.Inc()

	// For QoS 0, don't wait for confirmation
	if p.qos == 0 {
		metrics.MQTTPublishSuccessTotal.Inc()
		latency := time.Since(start)
		metrics.MQTTPublishLatency.Observe(latency.Seconds())
		return nil
	}

	// For QoS 1 and 2, handle asynchronously
	go func() {
		retryCount := 0
		for {
			if token.WaitTimeout(p.timeout) {
				if token.Error() == nil {
					metrics.MQTTPublishSuccessTotal.Inc()
					latency := time.Since(start)
					metrics.MQTTPublishLatency.Observe(latency.Seconds())
					return
				}

				if retryCount >= maxRetries {
					p.log.Error("Failed to publish message after retries",
						zap.Error(token.Error()),
						zap.String("topic", p.topic),
						zap.Int("max_retries", maxRetries),
						zap.Duration("elapsed_time", time.Since(start)))
					metrics.MQTTPublishFailureTotal.Inc()
					return
				}

				retryCount++
				p.log.Warn("Failed to publish message, retrying",
					zap.Error(token.Error()),
					zap.String("topic", p.topic),
					zap.Int("retry_attempt", retryCount),
					zap.Int("max_retries", maxRetries))
				
				// Try to republish
				if client.IsConnected() {
					token = client.Publish(p.topic, byte(p.qos), false, payload)
					time.Sleep(time.Millisecond * 100) // Short backoff before retry
					continue
				}
			}

			if retryCount >= maxRetries {
				p.log.Error("Publish timeout after retries",
					zap.String("topic", p.topic),
					zap.Int("max_retries", maxRetries),
					zap.Duration("elapsed_time", time.Since(start)))
				metrics.MQTTPublishFailureTotal.Inc()
				return
			}

			retryCount++
			p.log.Warn("Publish timeout, retrying",
				zap.String("topic", p.topic),
				zap.Int("retry_attempt", retryCount),
				zap.Int("max_retries", maxRetries))
			
			if client.IsConnected() {
				token = client.Publish(p.topic, byte(p.qos), false, payload)
				continue
			}
		}
	}()

	return nil
}

// RunPublish starts the publishing process
func (p *Publisher) RunPublish() error {
	p.log.Info("Starting publish test",
		zap.String("topic", p.topic),
		zap.Int("qos", p.qos),
		zap.Int("count", p.count),
		zap.Int("interval", p.interval))

	// Create connection manager with auto reconnect enabled
	p.options.ConnectRetryInterval = 5 // 5 seconds retry interval
	p.options.ConnectTimeout = 30      // 30 seconds connect timeout
	
	connManager := NewConnectionManager(p.options, 0)
	if err := connManager.RunConnections(); err != nil {
		return err
	}
	defer connManager.DisconnectAll()

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
	publishCount := int64(0)
	stopRateTracker := make(chan struct{})
	
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastCount := atomic.LoadInt64(&publishCount)
		lastTime := time.Now()

		for {
			select {
			case <-stopRateTracker:
				return
			case <-ticker.C:
				currentCount := atomic.LoadInt64(&publishCount)
				currentTime := time.Now()
				duration := currentTime.Sub(lastTime).Seconds()
				rate := float64(currentCount-lastCount) / duration
				metrics.MQTTPublishActualRate.Set(rate)
				lastCount = currentCount
				lastTime = currentTime
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
		clientLimiter := rate.NewLimiter(rate.Every(time.Duration(p.interval)*time.Millisecond), burstSize)
		go func(c mqtt.Client, clientID int, limiter *rate.Limiter) {
			defer wg.Done()
			p.log.Debug("Publisher goroutine started",
				zap.Int("client_id", clientID),
				zap.Int("message_count", p.count/len(clients)))

			for msgNum := 0; msgNum < p.count/len(clients); msgNum++ {
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

					// Generate payload
					payload := p.generateRandomPayload()
					p.log.Debug("Publishing message",
						zap.Int("client_id", clientID),
						zap.Int("message_number", msgNum+1),
						zap.Int("payload_size", len(payload)))

					// Publish with retry
					if err := p.publishWithRetry(c, payload, 3); err != nil {
						p.log.Error("Failed to publish message after retries",
							zap.Error(err),
							zap.String("topic", p.topic),
							zap.Int("client_id", clientID),
							zap.Int("message_number", msgNum+1))
					} else {
						atomic.AddInt64(&publishCount, 1)
					}
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
	return nil
}