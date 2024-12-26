/*
Copyright 2024 NAME HERE EMAIL ADDRESS
*/
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"go.uber.org/zap"
)

const (
	FlagServers        = "servers"
	FlagUser           = "user"
	FlagPassword       = "pass"
	FlagClientNum      = "clientNum"
	FlagLogLevel       = "log-level"
	FlagCleanSession   = "clean"
	FlagKeepAlive      = "keepalive"
	FlagRetryConnect   = "num-retry-connect"
	FlagConnRate       = "connrate"
	FlagMetricsPort    = "metrics-port"
	FlagPprofPort      = "pprof-port"
	FlagClientPrefix   = "client-prefix"
	FlagConnectTimeout = "connect-timeout"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mqtt-benchmark",
	Short: "A MQTT benchmark tool",
	Long: `A benchmark tool for MQTT brokers that allows you to test various aspects
of MQTT broker performance including connection handling, publishing, and subscribing.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize logger
		logLevel, _ := cmd.Flags().GetString(FlagLogLevel)
		if err := logger.InitLogger(logLevel); err != nil {
			fmt.Printf("Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}

		// Start metrics server
		metricsPort, _ := cmd.Flags().GetInt(FlagMetricsPort)
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			addr := fmt.Sprintf(":%d", metricsPort)
			logger.GetLogger().Info("Starting metrics server", zap.String("addr", addr))
			if err := http.ListenAndServe(addr, mux); err != nil {
				logger.GetLogger().Error("Metrics server error", zap.Error(err))
			}
		}()

		// Start pprof server if port is specified
		pprofPort, _ := cmd.Flags().GetInt(FlagPprofPort)
		if pprofPort > 0 {
			go func() {
				addr := fmt.Sprintf(":%d", pprofPort)
				logger.GetLogger().Info("Starting pprof server", zap.String("addr", addr))
				if err := http.ListenAndServe(addr, nil); err != nil {
					logger.GetLogger().Error("Pprof server error", zap.Error(err))
				}
			}()
		}
	},
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.GetLogger().Info("Received signal, shutting down...")
		cancel()
	}()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.GetLogger().Error("Failed to execute command", zap.Error(err))
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mqtt-benchmark.yaml)")

	// Add persistent flags
	rootCmd.PersistentFlags().StringArrayP(FlagServers, "S", []string{"127.0.0.1:1883"}, "mqtt servers")
	rootCmd.PersistentFlags().StringP(FlagUser, "u", "", "mqtt server username")
	rootCmd.PersistentFlags().StringP(FlagPassword, "P", "", "mqtt server password")
	rootCmd.PersistentFlags().Uint32P(FlagClientNum, "c", 100, "mqtt client num")
	rootCmd.PersistentFlags().String(FlagLogLevel, "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringP(FlagClientPrefix, "n", "mqtt-benchmark", "client ID prefix")

	// Add common MQTT connection flags
	rootCmd.PersistentFlags().BoolP(FlagCleanSession, "L", true, "clean session")
	rootCmd.PersistentFlags().Int(FlagKeepAlive, 60, "keepalive interval in seconds")
	rootCmd.PersistentFlags().Int(FlagRetryConnect, 0, "number of times to retry establishing a connection before giving up")
	rootCmd.PersistentFlags().IntP(FlagConnRate, "R", 0, "connection rate(/s), default: 0")
	rootCmd.PersistentFlags().Int(FlagConnectTimeout, 5, "connect timeout in seconds, default: 5")

	// Add metrics flag
	rootCmd.PersistentFlags().Int(FlagMetricsPort, 2112, "Port to expose Prometheus metrics")
	rootCmd.PersistentFlags().Int(FlagPprofPort, 0, "pprof port, 0 means disabled")
}

func fillMqttOptions(cmd *cobra.Command) *mqtt.OptionsCtx {
	ctx, cancel := context.WithCancel(cmd.Context())
	o := &mqtt.OptionsCtx{
		Context:    ctx,
		CancelFunc: cancel,
	}
	var err error
	if o.Servers, err = cmd.Flags().GetStringArray(FlagServers); err != nil {
		panic(err)
	}

	if o.User, err = cmd.Flags().GetString(FlagUser); err != nil {
		panic(err)
	}

	if o.Password, err = cmd.Flags().GetString(FlagPassword); err != nil {
		panic(err)
	}

	if o.ClientNum, err = cmd.Flags().GetUint32(FlagClientNum); err != nil {
		panic(err)
	}

	if o.ClientPrefix, err = cmd.Flags().GetString(FlagClientPrefix); err != nil {
		panic(err)
	}

	if o.CleanSession, err = cmd.Flags().GetBool(FlagCleanSession); err != nil {
		panic(err)
	}

	if o.KeepAliveSeconds, err = cmd.Flags().GetInt(FlagKeepAlive); err != nil {
		panic(err)
	}

	if o.ConnRate, err = cmd.Flags().GetInt(FlagConnRate); err != nil {
		panic(err)
	}

	if o.ConnectTimeout, err = cmd.Flags().GetInt(FlagConnectTimeout); err != nil {
		panic(err)
	}

	numRetry, err := cmd.Flags().GetInt(FlagRetryConnect)
	if err != nil {
		panic(err)
	}
	o.ConnectRetry = numRetry > 0
	o.ConnectRetryInterval = numRetry

	return o
}
