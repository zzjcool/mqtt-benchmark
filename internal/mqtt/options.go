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
	WaitForClients       bool // Whether to wait for other clients to be ready
	Inflight            int  // Maximum inflight messages for QoS 1 and 2
	WriteTimeout        int  // Write timeout in seconds

	// TLS Configuration
	CaFile     string // Path to CA certificate file
	CertFile   string // Path to client certificate file
	KeyFile    string // Path to client key file
	SkipVerify bool   // Skip server certificate verification

	OnConnectAttempt func(broker *url.URL, tlsCfg *tls.Config) *tls.Config
	OnConnect        func(client mqtt.Client, idx uint32)

	newClientFunc NewClientFunc
}
