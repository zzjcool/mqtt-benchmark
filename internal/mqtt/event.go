package mqtt

import (
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"go.uber.org/zap"
)

// EventExecutor handles MQTT client connection/disconnection events testing
type EventExecutor struct {
	optionsCtx      *OptionsCtx
	log             *zap.Logger
	waitTime        time.Duration
	subscriber      *ConnectionManager
	connection      *ConnectionManager
	connectCount    int
	disconnectCount int
}

// NewEventExecutor creates a new EventExecutor
func NewEventExecutor(options *OptionsCtx, waitTimeSeconds int) *EventExecutor {
	return &EventExecutor{
		optionsCtx: options,
		log:        logger.GetLogger(),
		waitTime:   time.Duration(waitTimeSeconds) * time.Second,
	}
}

// RunEventTest executes the event test
func (e *EventExecutor) RunEventTest() error {
	e.log.Info("Starting event test",
		zap.Uint32("client_num", e.optionsCtx.ClientNum))

	// Setup event subscriber
	subOptions := e.optionsCtx.Copy()
	subOptions.ClientPrefix = "eventSub"
	subOptions.ClientNum = 1
	subOptions.AfterAllClientsReady = func(activeClients []mqtt.Client) {
		for _, client := range activeClients {
			client.Subscribe("$events/#", 0, e.handleEventMessage)
		}
	}
	e.subscriber = NewConnectionManager(subOptions, 0)

	err := e.subscriber.RunConnections()
	if err != nil {
		return err
	}

	connOption := e.optionsCtx.Copy()
	connOption.ClientPrefix = "eventConn"
	e.connection = NewConnectionManager(connOption, 0)
	e.connection.RunConnections()

	time.Sleep(e.waitTime)
	e.connection.DisconnectAll()
	time.Sleep(e.waitTime)

	// Report results
	e.log.Info("Event test completed",
		zap.Uint32("expected_connections", e.optionsCtx.ClientNum),
		zap.Int("actual_connections", e.connectCount),
		zap.Uint32("expected_disconnections", e.optionsCtx.ClientNum),
		zap.Int("actual_disconnections", e.disconnectCount))

	if e.connectCount != int(e.optionsCtx.ClientNum) || e.disconnectCount != int(e.optionsCtx.ClientNum) {
		e.log.Info("Event count mismatch",
			zap.Uint32("expected_connections", e.optionsCtx.ClientNum),
			zap.Int("actual_connections", e.connectCount),
			zap.Uint32("expected_disconnections", e.optionsCtx.ClientNum),
			zap.Int("actual_disconnections", e.disconnectCount))
	}

	return nil
}

func (e *EventExecutor) handleEventMessage(_ mqtt.Client, msg mqtt.Message) {
	switch msg.Topic() {
	case "$events/client_connected":
		e.connectCount++
	case "$events/client_disconnected":
		e.disconnectCount++
	}
}
