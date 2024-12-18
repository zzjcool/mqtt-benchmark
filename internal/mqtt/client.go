package mqtt

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
)

// NewClientOptions creates a new MQTT client options with the given configuration
func (o *Options) NewClientOptions() *mqtt.ClientOptions {
	options := mqtt.NewClientOptions().
		SetAutoReconnect(o.AutoReconnect).
		SetKeepAlive(time.Second * time.Duration(o.KeepAliveSeconds)).
		SetUsername(o.User).
		SetPassword(o.Password).
		SetCleanSession(o.CleanSession).
		SetConnectTimeout(time.Second * 5).
		SetWriteTimeout(time.Second * 5).
		SetMaxReconnectInterval(time.Second * 10).
		SetProtocolVersion(4)

	// Configure connection lost handler
	options.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		broker := options.Servers[0].String()
		metrics.MQTTConnections.WithLabelValues(broker).Dec()
		metrics.MQTTConnectionErrors.WithLabelValues(broker, "connection_lost").Inc()
		
		// 添加 debug 日志
		logger.GetLogger().Debug("MQTT connection lost",
			zap.String("client_id", options.ClientID),
			zap.String("broker", broker),
			zap.Error(err))
	})

	options.SetOnConnectHandler(func(client mqtt.Client) {
		broker := options.Servers[0].String()
		metrics.MQTTConnections.WithLabelValues(broker).Inc()
	})

	for _, server := range o.Servers {
		options.AddBroker(server)
	}
	return options
}

func (o *Options) NewClientOptionsIterator(yield func(*mqtt.ClientOptions) bool) {
	for i := uint16(0); i < o.ClientNum; i++ {
		client := o.NewClientOptions()
		client.SetClientID(o.ClientPrefix + fmt.Sprint(i))
		if !yield(client) {
			break
		}
	}
}
