package mqtt

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
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
	options       *Options
	log           *zap.Logger
	activeClients []mqtt.Client
	clientsMutex  sync.Mutex
	keepTime      int
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(options *Options, keepTime int) *ConnectionManager {
	return &ConnectionManager{
		options:  options,
		log:      logger.GetLogger(),
		keepTime: keepTime,
	}
}

// RunConnections establishes MQTT connections based on the configured options
func (m *ConnectionManager) RunConnections() error {
	// 设置连接速率限制指标
	metrics.MQTTConnectionRateLimit.Set(float64(m.options.ConnRate))
	metrics.MQTTConnectionPoolSize.Set(float64(m.options.ClientNum))

	// 记录开始时间
	startTime := time.Now()
	var successCount, failCount atomic.Int64

	// 为每个 broker 创建连接计数器
	brokerConnections := make(map[string]*atomic.Int64)
	for _, server := range m.options.Servers {
		brokerConnections[server] = &atomic.Int64{}
	}

	// 创建工作池
	numWorkers := 100
	if m.options.ConnRate > 0 {
		numWorkers = m.options.ConnRate
	}

	// 创建限速器
	var limiter *rate.Limiter
	if m.options.ConnRate > 0 {
		limiter = rate.NewLimiter(rate.Limit(m.options.ConnRate), 1)
	}

	// 用于生成唯一的客户端 ID
	var clientCounter atomic.Uint32

	// 创建一个通道用于跟踪活动连接
	resultCh := make(chan ConnectionResult, m.options.ClientNum)

	// 创建等待组以跟踪所有工作线程
	var wg sync.WaitGroup

	// 启动工作线程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// 获取下一个客户端 ID
				clientIndex := clientCounter.Add(1) - 1
				if clientIndex >= uint32(m.options.ClientNum) {
					return
				}

				// 如果设置了速率限制，等待令牌
				if limiter != nil {
					err := limiter.Wait(context.Background())
					if err != nil {
						m.log.Error("Rate limiter error", zap.Error(err))
						return
					}
				}

				// 创建新的客户端选项
				opts := m.options.NewClientOptions()
				opts.SetClientID(fmt.Sprintf("%s%d", m.options.ClientPrefix, clientIndex))

				// 记录连接开始时间
				connectStart := time.Now()

				// 获取 broker 地址
				broker := opts.Servers[0].String()

				// 添加 debug 日志：开始连接
				m.log.Debug("Attempting MQTT connection",
					zap.String("client_id", opts.ClientID),
					zap.String("broker", broker))

				client := mqtt.NewClient(opts)
				token := client.Connect()

				// 记录连接尝试
				metrics.MQTTConnectionAttempts.WithLabelValues(broker, "attempt").Inc()

				if token.Wait() && token.Error() != nil {
					failCount.Add(1)
					metrics.MQTTConnectionAttempts.WithLabelValues(broker, "failure").Inc()

					// 根据错误类型记录
					errType := "network"
					if token.Error().Error() == "connection timed out" {
						errType = "timeout"
					}
					metrics.MQTTConnectionErrors.WithLabelValues(broker, errType).Inc()

					// 添加 debug 日志：连接失败
					m.log.Debug("MQTT connection failed",
						zap.String("client_id", opts.ClientID),
						zap.String("broker", broker),
						zap.String("error_type", errType),
						zap.Error(token.Error()),
						zap.Duration("attempt_duration", time.Since(connectStart)))

					resultCh <- ConnectionResult{
						ClientID: opts.ClientID,
						Broker:   broker,
						Success:  false,
						Error:    token.Error(),
					}
					continue
				}

				// 记录连接成功
				successCount.Add(1)
				metrics.MQTTConnectionAttempts.WithLabelValues(broker, "success").Inc()
				brokerConnections[broker].Add(1)
				metrics.MQTTNewConnections.WithLabelValues(broker).Inc()

				// 添加到活跃客户端列表
				m.clientsMutex.Lock()
				m.activeClients = append(m.activeClients, client)
				m.clientsMutex.Unlock()

				// 添加 debug 日志：连接成功
				m.log.Debug("MQTT connection established",
					zap.String("client_id", opts.ClientID),
					zap.String("broker", broker),
					zap.Duration("connect_time", time.Since(connectStart)))

				// 记录连接时间
				connectDuration := time.Since(connectStart)
				metrics.MQTTConnectionTime.WithLabelValues(broker).Observe(connectDuration.Seconds())

				resultCh <- ConnectionResult{
					ClientID: opts.ClientID,
					Broker:   broker,
					Success:  true,
				}
			}
		}()
	}

	// 等待所有连接完成
	wg.Wait()
	close(resultCh)

	// 处理所有结果
	var results []ConnectionResult
	for result := range resultCh {
		results = append(results, result)
	}

	// 计算实际连接速率
	duration := time.Since(startTime)
	totalConnections := successCount.Load()

	// 每个 broker 的连接数
	brokerConnectionsCount := make(map[string]int64)
	for _, server := range m.options.Servers {
		brokerConnectionsCount[server] = brokerConnections[server].Load()
	}

	// 更新连接池指标
	metrics.MQTTConnectionPoolActive.Set(float64(totalConnections))

	m.log.Info("Connection test completed",
		zap.Duration("duration", duration),
		zap.Uint16("total_clients", m.options.ClientNum),
		zap.Int64("connected", successCount.Load()),
		zap.Int64("failed", failCount.Load()),
		zap.Float64("total_connections_per_second", float64(totalConnections)/duration.Seconds()),
	)

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
		m.log.Debug("Disconnecting client",
			zap.Int("client_index", i),
			zap.String("client_id", opts.ClientID()))
		client.Disconnect(250) // 250ms 超时
	}

	m.log.Info("All clients disconnected")
	return nil
}
