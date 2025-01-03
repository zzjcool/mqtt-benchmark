package mqtt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"bytes"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
)

// Subscriber handles MQTT message subscription operations
type Subscriber struct {
	wg             sync.WaitGroup
	optionsCtx     *OptionsCtx
	topicGenerator *TopicGenerator
	qos            int
	timeout        time.Duration
	msgCount       int64
	log            *zap.Logger
	parseTimestamp bool // Parse timestamp from payload
}

// NewSubscriber creates a new Subscriber
func NewSubscriber(options *OptionsCtx, topic string, topicNum int, clientIndex uint32, qos int) *Subscriber {
	return &Subscriber{
		optionsCtx:     options,
		topicGenerator: NewTopicGenerator(topic, topicNum, clientIndex),
		qos:            qos,
		timeout:        5 * time.Second,
		log:            logger.GetLogger(),
		parseTimestamp: false,
	}
}

// SetTimeout sets the subscription timeout duration
func (s *Subscriber) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// SetParseTimestamp sets whether to parse timestamp from payload
func (s *Subscriber) SetParseTimestamp(parseTimestamp bool) {
	s.parseTimestamp = parseTimestamp
}

// RunSubscribe starts the subscription process
func (s *Subscriber) RunSubscribe() error {
	s.log.Info("Starting subscribe test",
		zap.String("topic", s.topicGenerator.GetTopic()),
		zap.Int("qos", s.qos))

	// Create connection manager with auto reconnect enabled
	s.optionsCtx.ConnectRetryInterval = 5 // 5 seconds retry interval
	s.optionsCtx.OnConnect = s.handleClientConnect
	s.optionsCtx.OnConnectionLost = s.handleClientConnectionLost
	s.optionsCtx.OnFirstConnect = s.handleClientConnectFirst

	s.report()
	connManager := NewConnectionManager(s.optionsCtx, 0)
	if err := connManager.RunConnections(); err != nil {
		return err
	}

	defer connManager.DisconnectAll()
	done := make(chan struct{})

	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.log.Info("Subscribe test completed",
			zap.Int64("total_messages_received", atomic.LoadInt64(&s.msgCount)))
	case <-s.optionsCtx.Done():
	}

	s.log.Info("Subscribe test completed",
		zap.Int64("total_messages_received", atomic.LoadInt64(&s.msgCount)))

	if errors.Is(s.optionsCtx.Err(), context.Canceled) {
		return nil
	}
	return s.optionsCtx.Err()
}

func (s *Subscriber) handleClientConnectionLost(client mqtt.Client, err error) {
	// op := client.OptionsReader()
	// TODO wg.Done() let client can quit
	if s.optionsCtx.IsDropConnection(client) {
		s.wg.Done()
	}
}

func (s *Subscriber) handleClientConnectFirst(client mqtt.Client, idx uint32) {
	s.wg.Add(1)
}

func (s *Subscriber) handleClientConnect(client mqtt.Client, idx uint32) {
	// Create a new TopicGenerator for each client with its own clientID
	clientTopicGen := NewTopicGenerator(s.topicGenerator.TopicTemplate, s.topicGenerator.TopicNum, idx)
	s.log.Debug("Subscriber goroutine started",
		zap.Uint32("client_id", idx))
	metrics.MQTTActiveSubscribers.Inc()

	// Create message handler
	messageHandler := func(c mqtt.Client, msg mqtt.Message) {
		atomic.AddInt64(&s.msgCount, 1)
		metrics.MQTTMessagesReceived.Inc()
		metrics.MQTTMessageReceiveRate.Inc()
		metrics.MQTTMessageQosDistribution.WithLabelValues(fmt.Sprintf("%d", msg.Qos())).Inc()
		metrics.MQTTMessagePayloadSize.Observe(float64(len(msg.Payload())))

		latency := float64(0)
		// Calculate latency if timestamp is in payload
		if len(msg.Payload()) > 0 && s.parseTimestamp {
			// Try to parse timestamp from the beginning of payload
			payload := msg.Payload()
			if i := bytes.IndexByte(payload, '|'); i > 0 {
				if ts, err := time.Parse(time.RFC3339Nano, string(payload[:i])); err == nil {
					latency = time.Since(ts).Seconds()
					metrics.MQTTMessageReceiveLatency.Observe(latency)
				} else {
					s.log.Error("Failed to parse timestamp",
						zap.Uint32("client_id", idx),
						zap.String("topic", msg.Topic()),
						zap.Int("qos", int(msg.Qos())),
						zap.Int("payload_size", len(msg.Payload())),
						zap.Error(err))
				}
			}
		}

		s.log.Debug("Message received",
			zap.Uint32("client_id", idx),
			zap.String("topic", msg.Topic()),
			zap.Int("qos", int(msg.Qos())),
			zap.Int("payload_size", len(msg.Payload())),
			zap.Float64("latency_ms", latency*1000),
		)
	}

	// Subscribe to all topics
	topics := clientTopicGen.GetTopics()
	for _, topic := range topics {
		token := client.Subscribe(topic, byte(s.qos), messageHandler)
		if token.WaitTimeout(s.timeout) {
			if err := token.Error(); err != nil {
				metrics.MQTTSubscriptionErrors.WithLabelValues(topic, "subscription_failed").Inc()
				s.log.Error("Failed to subscribe",
					zap.Uint32("client_id", idx),
					zap.Error(err))
				return
			}
			s.log.Debug("Successfully subscribed",
				zap.Uint32("client_id", idx),
				zap.String("topic", topic))
		} else {
			metrics.MQTTSubscriptionErrors.WithLabelValues(topic, "timeout").Inc()
			s.log.Error("Subscription timeout",
				zap.Uint32("client_id", idx))
			return
		}
	}
}

func (s *Subscriber) report() {
	// Initialize metrics
	metrics.MQTTMessagesReceived.Add(0)
	// Start a goroutine to log message count every second
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		preMessagesReceived := uint64(metrics.GetCounterValue(metrics.MQTTMessagesReceived))
		preMessagesPayloadSize := uint64(metrics.GetHistogramValue(metrics.MQTTMessagePayloadSize))
		preMessagesLatency := metrics.GetHistogramValue(metrics.MQTTMessageReceiveLatency)
		for {
			select {
			case <-s.optionsCtx.Done():
				return
			case <-ticker.C:
				messagesReceived := uint64(metrics.GetCounterValue(metrics.MQTTMessagesReceived))
				messagesPayloadSize := uint64(metrics.GetHistogramValue(metrics.MQTTMessagePayloadSize))
				messagesLatency := metrics.GetHistogramValue(metrics.MQTTMessageReceiveLatency)

				rate := messagesReceived - preMessagesReceived
				if rate == 0 {
					continue
				}
				averagePayload := uint64(0)
				avgLatency := float64(0)
				if rate != 0 {
					averagePayload = (messagesPayloadSize - preMessagesPayloadSize) / rate
					avgLatency = (messagesLatency - preMessagesLatency) / float64(rate) * 1000
				}
				s.log.Info("Received messages",
					zap.Uint64("rate", rate),
					zap.Uint64("received_total", messagesReceived),
					zap.Uint64("average_payload", averagePayload),
					zap.Uint16("consume_latency_ms", uint16(avgLatency)),
				)

				preMessagesReceived = messagesReceived
				preMessagesPayloadSize = messagesPayloadSize
				preMessagesLatency = messagesLatency
			}
		}
	}()
}
