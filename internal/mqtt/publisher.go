package mqtt

import (
	"context"
	"crypto/rand"
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
func (p *Publisher) publishWithRetry(client mqtt.Client, payload []byte, topicGen *TopicGenerator, wg *sync.WaitGroup) error {
	topic := topicGen.NextTopic()
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
		retryCount := 0
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout*4) // 总超时时间为单次超时的4倍
		defer cancel()

		for retryCount <= 3 {
			select {
			case <-ctx.Done():
				p.log.Error("Publish operation timed out",
					zap.String("topic", topic),
					zap.Int("max_retries", 3),
					zap.Duration("elapsed_time", time.Since(startTime)))
				metrics.MQTTPublishFailureTotal.Inc()
				return
			default:
				if token.WaitTimeout(p.timeout) {
					if token.Error() == nil {
						metrics.MQTTPublishSuccessTotal.Inc()
						latency := time.Since(startTime)
						metrics.MQTTPublishLatency.Observe(latency.Seconds())
						return
					}
					metrics.MQTTPublishFailureTotal.Inc()
					if retryCount >= 3 {
						p.log.Error("Failed to publish message after retries",
							zap.Error(token.Error()),
							zap.String("topic", topic),
							zap.Int("max_retries", 3),
							zap.Duration("elapsed_time", time.Since(startTime)))

						return
					}

					retryCount++
					backoff := time.Duration(retryCount*retryCount) * 100 * time.Millisecond // 指数退避
					p.log.Warn("Failed to publish message, retrying",
						zap.Error(token.Error()),
						zap.String("topic", topic),
						zap.Int("retry_attempt", retryCount),
						zap.Int("max_retries", 3),
						zap.Duration("backoff", backoff))

					// 在重试前等待一段时间
					time.Sleep(backoff)

					// 检查客户端连接状态
					if !client.IsConnected() {
						p.log.Warn("Client disconnected, stopping retries",
							zap.String("topic", topic))
						return
					}

					// 重新发布消息
					token = client.Publish(topic, byte(p.qos), p.retain, payload)
					metrics.MQTTPublishTotal.Inc()
				} else {
					// 发布超时
					if retryCount >= 3 {
						p.log.Error("Publish timeout after retries",
							zap.String("topic", topic),
							zap.Int("max_retries", 3),
							zap.Duration("elapsed_time", time.Since(startTime)))
						return
					}

					retryCount++
					backoff := time.Duration(retryCount*retryCount) * 100 * time.Millisecond
					p.log.Warn("Publish timeout, retrying",
						zap.String("topic", topic),
						zap.Int("retry_attempt", retryCount),
						zap.Int("max_retries", 3),
						zap.Duration("backoff", backoff),
						zap.Int64("timeout_seconds", int64(p.timeout/time.Second)))

					time.Sleep(backoff)

					if !client.IsConnected() {
						p.log.Warn("Client disconnected, stopping retries",
							zap.String("topic", topic))
						return
					}
					token = client.Publish(topic, byte(p.qos), p.retain, payload)
					metrics.MQTTPublishTotal.Inc()
				}
			}
		}
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
	publishCount := int64(0)
	stopRateTracker := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastCount := atomic.LoadInt64(&publishCount)
		lastTime := time.Now()
		prePublishSuccessTotal := uint64(0)
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

				connectedCount := uint32(metrics.GetGaugeVecValue(metrics.MQTTConnections, p.options.Servers...))

				publishSuccessTotal := uint64(metrics.GetCounterValue(metrics.MQTTPublishSuccessTotal))
				p.log.Info("Publishing at rate",
					zap.Uint64("rate", publishSuccessTotal-prePublishSuccessTotal),
					zap.Uint64("publish_success_total", publishSuccessTotal),
					zap.Uint32("connected", connectedCount))
				prePublishSuccessTotal = publishSuccessTotal
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

					// Generate payload
					payload := p.generateRandomPayload()
					topic := clientTopicGen.NextTopic()
					p.log.Debug("Publishing message",
						zap.String("topic", topic),
						zap.Int("client_id", clientID),
						zap.Int("message_number", msgNum+1),
						zap.Int("payload_size", len(payload)))

					// Publish with retry
					if err := p.publishWithRetry(c, payload, clientTopicGen, &wg); err != nil {
						p.log.Error("Failed to publish message after retries",
							zap.Error(err),
							zap.String("topic", topic),
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

	// Wait a short time to ensure all QoS 1/2 messages are acknowledged
	time.Sleep(100 * time.Millisecond)

	// Disconnect all clients
	connManager.DisconnectAll()
	return nil
}
