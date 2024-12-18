/*
Copyright 2024 NAME HERE EMAIL ADDRESS
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	"go.uber.org/zap"
	"runtime"
)

const (
	FlagServers         = "servers"
	FlagUser           = "user"
	FlagPassword       = "pass"
	FlagClientNum      = "clientNum"
	FlagLogLevel       = "log-level"
	FlagCleanSession   = "clean"
	FlagKeepAlive      = "keepalive"
	FlagRetryConnect   = "num-retry-connect"
	FlagConnRate       = "connrate"
	FlagMetricsPort    = "metrics-port"
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
			http.Handle("/metrics", promhttp.Handler())
			addr := fmt.Sprintf(":%d", metricsPort)
			logger.GetLogger().Info("Starting metrics server", zap.String("addr", addr))
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.GetLogger().Error("Metrics server error", zap.Error(err))
			}
		}()

		// Start resource monitoring
		monitorResourceUsage(time.Second)
	},
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mqtt-benchmark.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Add persistent flags
	rootCmd.PersistentFlags().StringArrayP(FlagServers, "s", []string{"127.0.0.1:1883"}, "mqtt servers")
	rootCmd.PersistentFlags().StringP(FlagUser, "u", "", "mqtt server username")
	rootCmd.PersistentFlags().StringP(FlagPassword, "P", "", "mqtt server password")
	rootCmd.PersistentFlags().Uint16P(FlagClientNum, "c", 100, "mqtt client num")
	rootCmd.PersistentFlags().String(FlagLogLevel, "info", "log level (debug, info, warn, error)")
	
	// Add common MQTT connection flags
	rootCmd.PersistentFlags().BoolP(FlagCleanSession, "C", true, "clean session")
	rootCmd.PersistentFlags().Int(FlagKeepAlive, 60, "keepalive interval in seconds")
	rootCmd.PersistentFlags().Int(FlagRetryConnect, 0, "number of times to retry establishing a connection before giving up")
	rootCmd.PersistentFlags().IntP(FlagConnRate, "R", 0, "connection rate(/s), default: 0")

	// Add metrics flag
	rootCmd.PersistentFlags().Int(FlagMetricsPort, 2112, "Port to expose Prometheus metrics")
}

// monitorResourceUsage periodically collects and updates system resource metrics
func monitorResourceUsage(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		var m runtime.MemStats
		for range ticker.C {
			runtime.ReadMemStats(&m)
			metrics.MQTTMemoryUsage.Set(float64(m.Alloc))
			metrics.MQTTCPUUsage.Set(float64(runtime.NumGoroutine()) / 100.0)
		}
	}()
}

func fillMqttOptions(cmd *cobra.Command) *mqtt.Options {
	o := &mqtt.Options{}
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

	if o.ClientNum, err = cmd.Flags().GetUint16(FlagClientNum); err != nil {
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

	numRetry, err := cmd.Flags().GetInt(FlagRetryConnect)
	if err != nil {
		panic(err)
	}
	o.AutoReconnect = numRetry > 0

	return o
}
