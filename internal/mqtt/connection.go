package mqtt

import (
	"context"
	"crypto/tls"
	"errors"
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
	optionsCtx    *OptionsCtx
	log           *zap.Logger
	activeClients []mqtt.Client
	clientsMutex  sync.Mutex
	keepTime      int
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(options *OptionsCtx, keepTime int) *ConnectionManager {
	return &ConnectionManager{
		optionsCtx: options,
		log:        logger.GetLogger(),
		keepTime:   keepTime,
	}
}

// RunConnections establishes MQTT connections based on the configured options
func (m *ConnectionManager) RunConnections() error {
	// 设置连接速率限制指标
	metrics.MQTTConnectionRateLimit.Set(float64(m.optionsCtx.ConnRate))

	// Create rate limiter for connection attempts
	limiter := rate.NewLimiter(rate.Limit(m.optionsCtx.ConnRate), 1)

	// Create MQTT clients
	m.clientsMutex.Lock()
	defer m.clientsMutex.Unlock()

	var wg sync.WaitGroup
	clientChan := make(chan mqtt.Client, m.optionsCtx.ClientNum)

	go func() {
		for {
			select {
			case client, ok := <-clientChan:
				if !ok {
					m.log.Debug("Client channel closed, stopping connection collection")
					return
				}
				m.log.Debug("New client connected")
				m.activeClients = append(m.activeClients, client)
			case <-m.optionsCtx.Done():
				m.log.Debug("Connection collection goroutine cancelled")
				return
			}
		}
	}()

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
			case <-m.optionsCtx.Done():
				m.log.Debug("Connection progress goroutine cancelled")
				return
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
		if err := limiter.Wait(m.optionsCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				m.log.Debug("Connection manager cancelled")
				return nil
			}
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
				if m.optionsCtx.WaitForClients {
					<-progressDone
				}
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
					atomic.AddUint32(&failedCount, 1)
					return
				}
				metrics.MQTTConnectionTime.Observe(time.Since(startTime).Seconds())
				clientChan <- client
			} else {
				m.log.Error("Connection timeout",
					zap.String("client_id", clientID))
				metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr, "failure").Inc()
				atomic.AddUint32(&failedCount, 1)
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(clientChan)
	close(progressDone)

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

		// Use context with timeout instead of sleep
		ctx := context.Background()
		if m.optionsCtx != nil {
			ctx = m.optionsCtx
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(m.keepTime)*time.Second)
		defer cancel()

		select {
		case <-timeoutCtx.Done():
			if err := timeoutCtx.Err(); err != context.DeadlineExceeded {
				return err
			}
		case <-m.optionsCtx.Done():
			return m.optionsCtx.Err()
		}
	} else {
		m.log.Info("Keeping connections alive forever. Press Ctrl+C to exit",
			zap.Int("active_connections", len(m.activeClients)))
		// Create a channel to handle signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigChan:
			m.log.Info("Received signal, shutting down...")
		case <-m.optionsCtx.Done():
			return m.optionsCtx.Err()
		}
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
		servers := opts.Servers()
		if len(servers) == 0 {
			m.log.Debug("Client has no servers configured",
				zap.Int("client_index", i),
				zap.String("client_id", opts.ClientID()))
			continue
		}
		broker := servers[0].String()
		m.log.Debug("Disconnecting client",
			zap.Int("client_index", i),
			zap.String("client_id", opts.ClientID()))
		client.Disconnect(250) // 250ms timeout
		metrics.MQTTConnections.WithLabelValues(broker).Dec()
	}

	m.log.Info("All clients disconnected")
	return nil
}
