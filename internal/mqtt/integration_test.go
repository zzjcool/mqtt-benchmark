package mqtt

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	mserver "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	server *mserver.Server
	port   int
}

func setupMQTTServer(t *testing.T) *testServer {
	// Find an available port
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create and start MQTT server
	srv := mserver.New(&mserver.Options{})
	_ = srv.AddHook(new(auth.AllowHook), nil) // Allow all connections

	// Create TCP listener
	tcpConfig := listeners.Config{
		ID:      "t1",
		Address: fmt.Sprintf(":%d", port),
	}
	err = srv.AddListener(listeners.NewTCP(tcpConfig))
	require.NoError(t, err)

	go func() {
		err := srv.Serve()
		if err != nil {
			t.Logf("MQTT server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	return &testServer{
		server: srv,
		port:   port,
	}
}

func setupTestOptions(t *testing.T, port int, clientNum uint32) *OptionsCtx {
	ctx, cancel := context.WithCancel(context.Background())
	options := &OptionsCtx{
		Context:          ctx,
		CancelFunc:       cancel,
		Servers:          []string{fmt.Sprintf("tcp://localhost:%d", port)},
		ClientNum:        clientNum,
		ClientPrefix:     "test",
		ConnRate:         100,
		KeepAliveSeconds: 60,
		QoS:              0,
		AutoReconnect:    true,
		CleanSession:     true,
		ConnectTimeout:   5,
		ConnectRetry:     true,
		WaitForClients:   true,
		Inflight:         100,
		WriteTimeout:     5,
		OnConnect:        func(client pahomqtt.Client, idx uint32) {},
		OnFirstConnect:   func(client pahomqtt.Client, idx uint32) {},
		OnConnectionLost: func(client pahomqtt.Client, err error) {},
	}
	return options
}

func TestIntegrationPublishSubscribe(t *testing.T) {
	// Setup embedded MQTT server
	mqttServer := setupMQTTServer(t)
	defer mqttServer.server.Close()

	// Create options for the test
	pubOptions := setupTestOptions(t, mqttServer.port, 1)
	pubOptions.ClientPrefix = "pub"
	defer pubOptions.CancelFunc()

	subOptions := setupTestOptions(t, mqttServer.port, 1)
	subOptions.ClientPrefix = "sub"
	defer subOptions.CancelFunc()

	const (
		topic        = "test/integration"
		messageCount = 10
		qos          = 1
		payloadSize  = 100
		rate         = 100.0 // messages per second
	)
	receivedMessages := make(chan struct{}, messageCount)

	// Create and start subscriber
	sub := NewSubscriber(subOptions, topic, 1, 0, qos)
	sub.afterMessageReceived = func(c pahomqtt.Client, msg pahomqtt.Message) {
		receivedMessages <- struct{}{}
	}
	sub.SetTimeout(5 * time.Second)

	go func() {
		err := sub.RunSubscribe()
		require.NoError(t, err)
	}()

	// Wait for subscriber to connect
	time.Sleep(200 * time.Millisecond)

	// Create and start publisher
	pub := NewPublisher(pubOptions, topic, 1, 0, "", payloadSize, qos, messageCount, rate)
	err := pub.RunPublish()
	require.NoError(t, err)

	// Wait for all messages
	timeout := time.After(10 * time.Second)
	received := 0
	for received < messageCount {
		select {
		case <-receivedMessages:
			received++
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received %d/%d", received, messageCount)
		}
	}

	assert.Equal(t, messageCount, received, "Should receive all published messages")
}
