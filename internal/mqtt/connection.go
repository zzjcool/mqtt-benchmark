package mqtt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"sync/atomic"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ConnectionResult represents the result of a connection attempt
type ConnectionResult struct {
	ClientID string
	Broker   string
	Success  bool
	Error    error
}

// ConnectionManager handles MQTT connection operations
type ConnectionManager struct {
	optionsCtx       *OptionsCtx
	log           *zap.Logger
	activeClients []mqtt.Client
	clientsMutex  sync.Mutex
	keepTime      int
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(options *OptionsCtx, keepTime int) *ConnectionManager {
	return &ConnectionManager{
		optionsCtx:  options,
		log:      logger.GetLogger(),
		keepTime: keepTime,
	}
}

// RunConnections establishes MQTT connections based on the configured options
func (m *ConnectionManager) RunConnections() error {
	// 设置连接速率限制指标
	metrics.MQTTConnectionRateLimit.Set(float64(m.optionsCtx.ConnRate))
	metrics.MQTTConnectionPoolSize.Set(float64(m.optionsCtx.ClientNum))

	// Create rate limiter for connection attempts
	limiter := rate.NewLimiter(rate.Limit(m.optionsCtx.ConnRate), 1)

	// Create MQTT clients
	m.clientsMutex.Lock()
	defer m.clientsMutex.Unlock()

	var wg sync.WaitGroup
	clientChan := make(chan mqtt.Client, m.optionsCtx.ClientNum)
	errorChan := make(chan error, m.optionsCtx.ClientNum)

	// Add progress tracking
	var failedCount uint32
	progressDone := make(chan struct{})

	// Start progress reporting goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var lastConnectedCount uint32
		for {
			select {
			case <-ticker.C:
				connectedCount := uint32(metrics.GetGaugeVecValue(metrics.MQTTConnections, m.optionsCtx.Servers...))
				remaining := m.optionsCtx.ClientNum - (connectedCount + failedCount)

				// Calculate connection rate (connections per second)
				connectionRate := connectedCount - lastConnectedCount
				lastConnectedCount = connectedCount

				m.log.Info("Connection progress",
					zap.Uint32("connected", connectedCount),
					zap.Uint32("failed", failedCount),
					zap.Uint32("remaining", remaining),
					zap.Uint32("conn_rate", connectionRate))
			case <-progressDone:
				m.log.Info("Connection progress done")
				return
			}
		}
	}()

	for i := uint32(0); i < m.optionsCtx.ClientNum; i++ {
		// Wait for rate limiter
		if err := limiter.Wait(context.Background()); err != nil {
			m.log.Error("Rate limiter error", zap.Error(err))
			continue
		}

		wg.Add(1)
		go func(index uint32) {
			defer wg.Done()

			// Create unique client ID
			clientID := fmt.Sprintf("%s-%d", m.optionsCtx.ClientPrefix, index)
			serverIndex := index % uint32(len(m.optionsCtx.Servers))
			serverAddr := m.optionsCtx.Servers[serverIndex]

			// Configure MQTT client options
			opts := mqtt.NewClientOptions().
				AddBroker(serverAddr).
				SetClientID(clientID).
				SetUsername(m.optionsCtx.User).
				SetPassword(m.optionsCtx.Password).
				SetCleanSession(m.optionsCtx.CleanSession).
				SetKeepAlive(time.Duration(m.optionsCtx.KeepAliveSeconds) * time.Second).
				SetMaxReconnectInterval(10 * time.Second).
				SetConnectTimeout(time.Duration(m.optionsCtx.ConnectTimeout) * time.Second).
				SetAutoReconnect(true).
				SetConnectRetry(true).
				SetConnectRetryInterval(time.Second).
				SetOrderMatters(false).
				SetWriteTimeout(5 * time.Second).
				SetMaxResumePubInFlight(1024)

			opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {

			})

			opts.OnReconnecting = func(c mqtt.Client, co *mqtt.ClientOptions) {
				m.log.Debug("Client reconnecting",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
				metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr, "failure").Inc()
			}

			opts.OnConnectAttempt = func(broker *url.URL, tlsCfg *tls.Config) *tls.Config {
				m.log.Debug("Client connecting",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
				if m.optionsCtx.OnConnectAttempt != nil {
					return m.optionsCtx.OnConnectAttempt(broker, tlsCfg)
				}
				return tlsCfg
			}

			// Set connect handler
			opts.OnConnect = func(c mqtt.Client) {
				m.log.Debug("Client connected",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
				metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr, "success").Inc()
				metrics.MQTTConnections.WithLabelValues(serverAddr).Inc()
				metrics.MQTTNewConnections.WithLabelValues(serverAddr).Inc()
				if m.optionsCtx.OnConnect != nil {
					m.optionsCtx.OnConnect(c, index)
				}
			}

			// Set connection lost handler
			opts.OnConnectionLost = func(c mqtt.Client, err error) {
				m.log.Debug("Client connection lost",
					zap.String("client_id", clientID),
					zap.Error(err))
				metrics.MQTTConnectionErrors.WithLabelValues(serverAddr, "connection_lost").Inc()
				metrics.MQTTConnections.WithLabelValues(serverAddr).Dec()
			}

			// Create and connect client
			client := mqtt.NewClient(opts)
			startTime := time.Now()
			token := client.Connect()
			if token.WaitTimeout(time.Duration(m.optionsCtx.ConnectTimeout) * time.Second) {
				if token.Error() != nil {
					m.log.Error("Failed to connect",
						zap.String("client_id", clientID),
						zap.Error(token.Error()))
					metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr, "failure").Inc()
					errorChan <- token.Error()
					atomic.AddUint32(&failedCount, 1)
					return
				}
				metrics.MQTTConnectionTime.Observe(time.Since(startTime).Seconds())
				clientChan <- client
			} else {
				m.log.Error("Connection timeout",
					zap.String("client_id", clientID))
				metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr, "failure").Inc()
				errorChan <- fmt.Errorf("connection timeout for client %s", clientID)
				atomic.AddUint32(&failedCount, 1)
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(clientChan)
	close(errorChan)
	close(progressDone)

	// Collect successful connections
	for client := range clientChan {
		m.activeClients = append(m.activeClients, client)
	}

	metrics.MQTTConnectionPoolActive.Set(float64(len(m.activeClients)))

	if len(m.activeClients) == 0 {
		return fmt.Errorf("no clients connected: %v", <-errorChan)
	}

	m.log.Info("All clients connected",
		zap.Int("total_clients", len(m.activeClients)))
	return nil
}

// KeepConnections maintains the connections for the specified duration
func (m *ConnectionManager) KeepConnections() error {
	if m.keepTime == 0 {
		return nil
	}

	if m.keepTime > 0 {
		m.log.Info("Keeping connections alive",
			zap.Int("seconds", m.keepTime),
			zap.Int("active_connections", len(m.activeClients)))
		time.Sleep(time.Duration(m.keepTime) * time.Second)
	} else {
		m.log.Info("Keeping connections alive forever. Press Ctrl+C to exit",
			zap.Int("active_connections", len(m.activeClients)))
		// 创建一个通道来处理信号
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		m.log.Info("Received signal, shutting down...")
	}

	return m.DisconnectAll()
}

// DisconnectAll disconnects all active clients
func (m *ConnectionManager) DisconnectAll() error {
	m.log.Info("Disconnecting all clients...",
		zap.Int("total_clients", len(m.activeClients)))

	m.clientsMutex.Lock()
	defer m.clientsMutex.Unlock()

	for i, client := range m.activeClients {
		opts := client.OptionsReader()
		broker := opts.Servers()[0].String()
		m.log.Debug("Disconnecting client",
			zap.Int("client_index", i),
			zap.String("client_id", opts.ClientID()))
		client.Disconnect(250) // 250ms 超时
		metrics.MQTTConnections.WithLabelValues(broker).Dec()
	}

	m.log.Info("All clients disconnected")
	return nil
}
