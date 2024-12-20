package mqtt

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
)

// Subscriber handles MQTT message subscription operations
type Subscriber struct {
	options  *Options
	topic    string
	qos      int
	timeout  time.Duration
	log      *zap.Logger
	msgCount int64
}

// NewSubscriber creates a new Subscriber
func NewSubscriber(options *Options, topic string, qos int) *Subscriber {
	return &Subscriber{
		options: options,
		topic:   topic,
		qos:     qos,
		timeout: 5 * time.Second,
		log:     logger.GetLogger(),
	}
}

// SetTimeout sets the subscription timeout duration
func (s *Subscriber) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// RunSubscribe starts the subscription process
func (s *Subscriber) RunSubscribe() error {
	s.log.Info("Starting subscribe test",
		zap.String("topic", s.topic),
		zap.Int("qos", s.qos))

	// Create connection manager with auto reconnect enabled
	s.options.ConnectRetryInterval = 5 // 5 seconds retry interval
	s.options.ConnectTimeout = 30      // 30 seconds connect timeout

	connManager := NewConnectionManager(s.options, 0)
	if err := connManager.RunConnections(); err != nil {
		return err
	}
	defer connManager.DisconnectAll()

	// Get active clients
	clients := connManager.activeClients
	if len(clients) == 0 {
		s.log.Error("No active clients available")
		return nil
	}

	// Initialize metrics
	metrics.MQTTMessagesReceived.Add(0)

	// Create error channel to track fatal errors
	errChan := make(chan error, len(clients))
	var wg sync.WaitGroup
	ctx := context.Background()

	// Subscribe with each client
	for i, client := range clients {
		wg.Add(1)
		go func(c mqtt.Client, clientID int) {
			defer wg.Done()
			s.log.Debug("Subscriber goroutine started",
				zap.Int("client_id", clientID))

			// Create message handler
			messageHandler := func(c mqtt.Client, msg mqtt.Message) {
				atomic.AddInt64(&s.msgCount, 1)
				metrics.MQTTMessagesReceived.Inc()
				s.log.Debug("Message received",
					zap.Int("client_id", clientID),
					zap.String("topic", msg.Topic()),
					zap.Int("qos", int(msg.Qos())),
					zap.Int("payload_size", len(msg.Payload())))
			}

			// Subscribe to topic
			token := c.Subscribe(s.topic, byte(s.qos), messageHandler)
			if token.WaitTimeout(s.timeout) {
				if err := token.Error(); err != nil {
					s.log.Error("Failed to subscribe",
						zap.Int("client_id", clientID),
						zap.Error(err))
					errChan <- err
					return
				}
				s.log.Debug("Successfully subscribed",
					zap.Int("client_id", clientID),
					zap.String("topic", s.topic))
			} else {
				err := fmt.Errorf("subscription timeout for client %d", clientID)
				s.log.Error("Subscription timeout",
					zap.Int("client_id", clientID))
				errChan <- err
				return
			}

			// Keep the goroutine running to receive messages
			<-ctx.Done()
		}(client, i)
	}

	// Wait for all subscribers to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			s.log.Error("Error during subscription", zap.Error(err))
		}
	}

	s.log.Info("Subscribe test completed",
		zap.Int64("total_messages_received", atomic.LoadInt64(&s.msgCount)))
	return nil
}
