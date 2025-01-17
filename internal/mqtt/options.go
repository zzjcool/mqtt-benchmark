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
	Inflight             int  // Maximum inflight messages for QoS 1 and 2
	WriteTimeout         int  // Write timeout in seconds

	// TLS Configuration
	CaCertFile     string // Path to CA certificate file
	CaKeyFile      string // Path to CA private key file for dynamic certificate generation
	ClientCertFile string // Path to client certificate file
	ClientKeyFile  string // Path to client key file
	SkipVerify     bool   // Skip server certificate verification

	OnConnectAttempt func(broker *url.URL, tlsCfg *tls.Config) *tls.Config
	OnFirstConnect   func(client mqtt.Client, idx uint32)
	BeforeConnect    func(client mqtt.Client, idx uint32)
	OnConnect        func(client mqtt.Client, idx uint32)
	OnConnectionLost func(client mqtt.Client, err error)

	AfterAllClientsReady func(activeClients []mqtt.Client)

	newClientFunc NewClientFunc
}

func (o *OptionsCtx) IsDropConnection(c mqtt.Client) bool {
	op := c.OptionsReader()
	if !op.AutoReconnect() {
		if !c.IsConnected() {
			return true
		}
	}
	return false
}

func (o *OptionsCtx) Copy() *OptionsCtx {
    newOpt := &OptionsCtx{
        Context:              o.Context,
        CancelFunc:           o.CancelFunc,
        Servers:              make([]string, len(o.Servers)),
        User:                 o.User,
        Password:             o.Password,
        KeepAliveSeconds:     o.KeepAliveSeconds,
        ClientNum:            o.ClientNum,
        ClientPrefix:         o.ClientPrefix,
        ConnRate:             o.ConnRate,
        QoS:                  o.QoS,
        Retain:               o.Retain,
        ClientIndex:          o.ClientIndex,
        AutoReconnect:        o.AutoReconnect,
        CleanSession:         o.CleanSession,
        ConnectRetryInterval: o.ConnectRetryInterval,
        ConnectTimeout:       o.ConnectTimeout,
        ConnectRetry:         o.ConnectRetry,
        WaitForClients:       o.WaitForClients,
        Inflight:             o.Inflight,
        WriteTimeout:         o.WriteTimeout,
        CaCertFile:           o.CaCertFile,
        CaKeyFile:            o.CaKeyFile,
        ClientCertFile:       o.ClientCertFile,
        ClientKeyFile:        o.ClientKeyFile,
        SkipVerify:           o.SkipVerify,
        OnConnectAttempt:     o.OnConnectAttempt,
        OnFirstConnect:       o.OnFirstConnect,
        BeforeConnect:        o.BeforeConnect,
        OnConnect:            o.OnConnect,
        OnConnectionLost:     o.OnConnectionLost,
        newClientFunc:        o.newClientFunc,
    }

    copy(newOpt.Servers, o.Servers)

    return newOpt
}