package mqtt

import (
	"context"
	"crypto/rand"
	"errors"
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
	log         *zap.Logger
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
		log:         logger.GetLogger(),
	}
}

// generateRandomPayload generates a random payload of specified size
func (p *Publisher) generateRandomPayload() []byte {
	if p.payload != "" {
		return []byte(p.payload)
	}
	payload := make([]byte, p.payloadSize)
	rand.Read(payload)
	return payload
}

// publishWithRetry attempts to publish a message with retry logic
func (p *Publisher) publishWithRetry(client mqtt.Client, payload []byte, maxRetries int) error {
	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		if !client.IsConnected() {
			p.log.Warn("Client disconnected, waiting for reconnection",
				zap.String("client_id", fmt.Sprintf("client-%d", retry)),
				zap.Int("retry_attempt", retry+1),
				zap.Int("max_retries", maxRetries))
			time.Sleep(time.Second) // Wait for potential reconnection
			continue
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

		// For QoS 1 and 2, wait for confirmation but with shorter timeout
		if token.WaitTimeout(time.Second) {
			if token.Error() != nil {
				lastErr = token.Error()
				p.log.Warn("Failed to publish message, retrying",
					zap.Error(lastErr),
					zap.String("topic", p.topic),
					zap.Int("retry_attempt", retry+1),
					zap.Int("max_retries", maxRetries),
					zap.Duration("elapsed_time", time.Since(start)))
				metrics.MQTTPublishFailureTotal.Inc()
				time.Sleep(time.Millisecond * 100) // Reduced backoff time
				continue
			}
			// Success
			metrics.MQTTPublishSuccessTotal.Inc()
			latency := time.Since(start)
			metrics.MQTTPublishLatency.Observe(latency.Seconds())
			return nil
		} else {
			lastErr = errors.New("publish timeout")
			p.log.Warn("Publish timeout, retrying",
				zap.String("topic", p.topic),
				zap.Int("retry_attempt", retry+1),
				zap.Int("max_retries", maxRetries),
				zap.Duration("elapsed_time", time.Since(start)))
			metrics.MQTTPublishFailureTotal.Inc()
			continue
		}
	}
	return lastErr
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
