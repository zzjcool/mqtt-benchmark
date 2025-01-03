package mqtt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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
	if options.newClientFunc == nil {
		options.newClientFunc = mqtt.NewClient
	}
	return &ConnectionManager{
		optionsCtx: options,
		log:        logger.GetLogger(),
		keepTime:   keepTime,
	}
}

func (m *ConnectionManager) SetNewClientFunc(newClientFunc NewClientFunc) {
	m.optionsCtx.newClientFunc = newClientFunc
}

// RunConnections establishes MQTT connections based on the configured options
func (m *ConnectionManager) RunConnections() error {
	if m.optionsCtx.ClientNum == 0 {
		m.log.Info("Skipping connection manager, no clients to connect")
		return errors.New("no active clients")
	}

	go func() {
		<-m.optionsCtx.Done()
		m.log.Debug("Connection manager cancelled")
		m.DisconnectAll()
	}()

	// Create rate limiter for connection attempts
	limiter := rate.NewLimiter(rate.Limit(m.optionsCtx.ConnRate), 1)

	var wg sync.WaitGroup

	progressDone := make(chan struct{})

	// Start progress reporting goroutine
	m.report(progressDone)

	onceConnected := sync.Once{}
	onceConnectedDone := make(chan bool)

	errorCh := make(chan error, 1)

	inflightCh := make(chan struct{}, m.optionsCtx.ConnRate)
	// Initialize inflightCh with tokens
	for i := 0; i < m.optionsCtx.ConnRate; i++ {
		inflightCh <- struct{}{}
	}

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
		select {
		case <-m.optionsCtx.Done():
			return nil
		case <-inflightCh:
		case err := <-errorCh:
			return err
		}

		wg.Add(1)
		go func(index uint32) {
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
				SetConnectTimeout(time.Duration(m.optionsCtx.ConnectTimeout) * time.Second).
				SetWriteTimeout(time.Duration(m.optionsCtx.WriteTimeout) * time.Second).
				SetAutoReconnect(m.optionsCtx.AutoReconnect).
				SetMaxResumePubInFlight(m.optionsCtx.Inflight).
				SetResumeSubs(true).
				SetConnectRetry(m.optionsCtx.ConnectRetry).
				SetConnectRetryInterval(time.Duration(m.optionsCtx.ConnectRetryInterval) * time.Second)

			// Configure TLS if needed
			if m.optionsCtx.CaCertFile != "" || m.optionsCtx.ClientCertFile != "" {
				tlsConfig, err := m.createTLSConfig(clientID)
				if err != nil {
					m.log.Error("Failed to create TLS config", zap.Error(err))
					errorCh <- err
					return
				}
				opts.SetTLSConfig(tlsConfig)
			}

			if m.optionsCtx.OnConnectAttempt != nil {
				opts.SetTLSConfig(m.optionsCtx.OnConnectAttempt(nil, opts.TLSConfig))
			}

			opts.OnReconnecting = func(c mqtt.Client, co *mqtt.ClientOptions) {
				m.log.Debug("Client reconnecting",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
			}

			opts.OnConnectAttempt = func(broker *url.URL, tlsCfg *tls.Config) *tls.Config {
				m.log.Debug("Client connecting",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
				if m.optionsCtx.OnConnectAttempt != nil {
					return m.optionsCtx.OnConnectAttempt(broker, tlsCfg)
				}
				metrics.MQTTConnectionAttempts.WithLabelValues(serverAddr).Inc()
				return tlsCfg
			}

			firstConnect := sync.Once{}

			// Set connect handler
			opts.OnConnect = func(c mqtt.Client) {
				m.log.Debug("Client connected",
					zap.String("client_id", clientID),
					zap.String("broker", serverAddr))
				metrics.MQTTConnections.WithLabelValues(serverAddr).Inc()
				metrics.MQTTNewConnections.WithLabelValues(serverAddr).Inc()
				onceConnected.Do(func() {
					close(onceConnectedDone)
				})
				if m.optionsCtx.WaitForClients {
					<-progressDone
				}

				firstConnect.Do(func() {
					m.clientsMutex.Lock()
					wg.Done()
					inflightCh <- struct{}{}
					m.activeClients = append(m.activeClients, c)
					m.clientsMutex.Unlock()
					if m.optionsCtx.OnFirstConnect != nil {
						m.optionsCtx.OnFirstConnect(c, index)
					}
				})

				if m.optionsCtx.OnConnect != nil {
					m.optionsCtx.OnConnect(c, index)
				}
			}

			// Set connection lost handler
			opts.OnConnectionLost = func(c mqtt.Client, err error) {
				m.log.Error("Client connection lost",
					zap.String("client_id", clientID),
					zap.Error(err))
				metrics.MQTTConnectionErrors.WithLabelValues(serverAddr, metrics.MQTTConnectionLostErrors).Inc()
				metrics.MQTTConnections.WithLabelValues(serverAddr).Dec()
				if m.optionsCtx.OnConnectionLost != nil {
					m.optionsCtx.OnConnectionLost(c, err)
				}
			}

			// Create and connect client
			client := m.optionsCtx.newClientFunc(opts)
			startTime := time.Now()
			token := client.Connect()

			waitfunc := func() bool { return token.WaitTimeout(time.Duration(m.optionsCtx.ConnectTimeout) * time.Second) }
			if m.optionsCtx.AutoReconnect {
				waitfunc = func() bool { return token.Wait() }
			}

			if waitfunc() {
				if token.Error() != nil {
					m.log.Error("Failed to connect",
						zap.String("client_id", clientID),
						zap.Error(token.Error()))
					metrics.MQTTConnectionErrors.WithLabelValues(serverAddr, metrics.MQTTConnectionNetworkErrors).Inc()
					errorCh <- token.Error()
					return
				}
				metrics.MQTTConnectionTime.Observe(time.Since(startTime).Seconds())
			} else {
				m.log.Error("Connection timeout",
					zap.String("client_id", clientID))
				metrics.MQTTConnectionErrors.WithLabelValues(serverAddr, metrics.MQTTConnectionTimeoutErrors).Inc()
				errorCh <- errors.New("connection timeout")
				return
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-m.optionsCtx.Done():
		return nil
	case <-done:
	case err := <-errorCh:
		return err
	}

	<-onceConnectedDone
	if len(m.activeClients) == 0 {
		return errors.New("no active clients")
	}
	close(progressDone)

	m.log.Info("All clients connected",
		zap.Int("total_clients", len(m.activeClients)))
	return nil
}

// report starts a goroutine to report connection progress
func (m *ConnectionManager) report(progressDone chan struct{}) {
	metrics.MQTTConnectionRateLimit.Set(float64(m.optionsCtx.ConnRate))
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
				remaining := m.optionsCtx.ClientNum - connectedCount

				// Calculate connection rate (connections per second)
				connectionRate := connectedCount - lastConnectedCount
				lastConnectedCount = connectedCount

				m.log.Info("Connection progress",
					zap.Uint32("connected", connectedCount),
					zap.Uint32("remaining", remaining),
					zap.Uint32("conn_rate", connectionRate))
			case <-progressDone:
				m.log.Info("Connection progress done")
				return
			}
		}
	}()
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

func generateClientCertificate(caKeyFile, caCertFile, clientID string) ([]byte, []byte, error) {
	// Read CA private key
	caKeyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA key file: %v", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, nil, errors.New("failed to decode CA private key")
	}
	caPrivKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA private key: %v", err)
	}

	// Read CA certificate
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA certificate file: %v", err)
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return nil, nil, errors.New("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// Generate client key pair
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate client key pair: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: clientID,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create client certificate
	clientCertDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &clientKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client certificate: %v", err)
	}

	// Encode certificate and private key in PEM format
	clientCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: clientCertDER,
	})

	clientKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
	})

	return clientCertPEM, clientKeyPEM, nil
}

func (m *ConnectionManager) createTLSConfig(clientID string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: m.optionsCtx.SkipVerify,
	}

	// Load CA certificate if provided
	if m.optionsCtx.CaCertFile != "" {
		caCert, err := os.ReadFile(m.optionsCtx.CaCertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Check if we need to generate client certificate
	if m.optionsCtx.CaKeyFile != "" && m.optionsCtx.CaCertFile != "" && m.optionsCtx.ClientCertFile == "" {
		// Generate dynamic client certificate
		certPEM, keyPEM, err := generateClientCertificate(m.optionsCtx.CaKeyFile, m.optionsCtx.CaCertFile, clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate client certificate: %v", err)
		}

		// Parse the generated certificate and key
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to parse generated certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else if m.optionsCtx.ClientCertFile != "" && m.optionsCtx.ClientKeyFile != "" {
		// Load existing client certificate and key
		cert, err := tls.LoadX509KeyPair(m.optionsCtx.ClientCertFile, m.optionsCtx.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
