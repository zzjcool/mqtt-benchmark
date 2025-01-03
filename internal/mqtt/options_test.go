package mqtt

import (
	"context"
	"crypto/tls"
	"net/url"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
)

func TestOptionsCtx(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *OptionsCtx
		validate func(*testing.T, *OptionsCtx)
	}{
		{
			name: "Default options",
			setup: func() *OptionsCtx {
				ctx, cancel := context.WithCancel(context.Background())
				return &OptionsCtx{
					Context:     ctx,
					CancelFunc:  cancel,
					Servers:     []string{"tcp://localhost:1883"},
					ClientNum:   1,
					QoS:         0,
					Inflight:    100,
					WriteTimeout: 30,
				}
			},
			validate: func(t *testing.T, opts *OptionsCtx) {
				assert.Equal(t, []string{"tcp://localhost:1883"}, opts.Servers)
				assert.Equal(t, uint32(1), opts.ClientNum)
				assert.Equal(t, 0, opts.QoS)
				assert.Equal(t, 100, opts.Inflight)
				assert.Equal(t, 30, opts.WriteTimeout)
			},
		},
		{
			name: "TLS options",
			setup: func() *OptionsCtx {
				ctx, cancel := context.WithCancel(context.Background())
				return &OptionsCtx{
					Context:        ctx,
					CancelFunc:     cancel,
					Servers:        []string{"tls://localhost:8883"},
					CaCertFile:     "ca.pem",
					ClientCertFile: "client.pem",
					ClientKeyFile:  "client.key",
					SkipVerify:     true,
				}
			},
			validate: func(t *testing.T, opts *OptionsCtx) {
				assert.Equal(t, []string{"tls://localhost:8883"}, opts.Servers)
				assert.Equal(t, "ca.pem", opts.CaCertFile)
				assert.Equal(t, "client.pem", opts.ClientCertFile)
				assert.Equal(t, "client.key", opts.ClientKeyFile)
				assert.True(t, opts.SkipVerify)
			},
		},
		{
			name: "Connection options",
			setup: func() *OptionsCtx {
				ctx, cancel := context.WithCancel(context.Background())
				return &OptionsCtx{
					Context:              ctx,
					CancelFunc:           cancel,
					KeepAliveSeconds:     30,
					ConnRate:             100,
					AutoReconnect:        true,
					CleanSession:         true,
					ConnectRetryInterval: 5,
					ConnectTimeout:       10,
					ConnectRetry:         true,
					WaitForClients:       true,
				}
			},
			validate: func(t *testing.T, opts *OptionsCtx) {
				assert.Equal(t, 30, opts.KeepAliveSeconds)
				assert.Equal(t, 100, opts.ConnRate)
				assert.True(t, opts.AutoReconnect)
				assert.True(t, opts.CleanSession)
				assert.Equal(t, 5, opts.ConnectRetryInterval)
				assert.Equal(t, 10, opts.ConnectTimeout)
				assert.True(t, opts.ConnectRetry)
				assert.True(t, opts.WaitForClients)
			},
		},
		{
			name: "Callback functions",
			setup: func() *OptionsCtx {
				ctx, cancel := context.WithCancel(context.Background())
				opts := &OptionsCtx{
					Context:    ctx,
					CancelFunc: cancel,
				}
				opts.OnConnectAttempt = func(broker *url.URL, tlsCfg *tls.Config) *tls.Config {
					return tlsCfg
				}
				opts.OnFirstConnect = func(client mqtt.Client, idx uint32) {}
				opts.newClientFunc = func(opts *mqtt.ClientOptions) mqtt.Client {
					return nil
				}
				return opts
			},
			validate: func(t *testing.T, opts *OptionsCtx) {
				assert.NotNil(t, opts.OnConnectAttempt)
				assert.NotNil(t, opts.OnFirstConnect)
				assert.NotNil(t, opts.newClientFunc)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.setup()
			tt.validate(t, opts)
			opts.CancelFunc() // Clean up context
		})
	}
}
