package mqtt

import (
	"context"
	"crypto/tls"
	"net/url"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type OptionsCtx struct {
	context.Context
	context.CancelFunc

	Servers              []string
	User                 string
	Password             string
	KeepAliveSeconds     int
	ClientNum            uint32
	ClientPrefix         string
	ConnRate             int
	QoS                  int
	Retain               bool
	ClientIndex          uint32
	AutoReconnect        bool
	CleanSession         bool
	ConnectRetryInterval int  // Seconds between connection retries
	ConnectTimeout       int  // Connection timeout in seconds
	ConnectRetry         bool // Whether to retry connection

	OnConnectAttempt func(broker *url.URL, tlsCfg *tls.Config) *tls.Config
	OnConnect        func(client mqtt.Client, idx uint32)
}
