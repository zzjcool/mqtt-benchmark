/*
Copyright 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	"github.com/zzjcool/mqtt-benchmark/internal/metrics"
	internalmqtt "github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"go.uber.org/zap"
)

var metricsPort int

// connCmd represents the conn command
var connCmd = &cobra.Command{
	Use:   "conn",
	Short: "Connect multiple MQTT clients to broker(s)",
	Long: `Connect multiple MQTT clients to one or more MQTT brokers with specified parameters.
This command allows you to test broker connection handling with various parameters like
connection rate, number of clients, and authentication settings.`,
	Run: func(cmd *cobra.Command, args []string) {
		log := logger.GetLogger()

		// Start metrics server in a goroutine
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			addr := fmt.Sprintf(":%d", metricsPort)
			log.Info("Starting metrics server", zap.String("addr", addr))
			if err := http.ListenAndServe(addr, nil); err != nil {
				log.Error("Metrics server error", zap.Error(err))
			}
		}()

		// 获取保持连接时间
		keepTime, _ := cmd.Flags().GetInt("keep-time")

		// 获取 MQTT 选项
		options := fillMqttOptions(cmd)

		// 创建连接管理器
		connManager := internalmqtt.NewConnectionManager(options, keepTime)

		// 启动资源监控
		monitorResourceUsage(time.Second)

		// 运行连接测试
		if err := connManager.RunConnections(); err != nil {
			log.Error("Failed to run connections", zap.Error(err))
			os.Exit(1)
		}

		// 保持连接（如果需要）
		if err := connManager.KeepConnections(); err != nil {
			log.Error("Failed to keep connections", zap.Error(err))
			os.Exit(1)
		}
	},
}

func monitorResourceUsage(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		var m runtime.MemStats
		for range ticker.C {
			runtime.ReadMemStats(&m)
			metrics.MQTTMemoryUsage.Set(float64(m.Alloc))

			// 这里我们只是简单地计算 CPU 使用率
			// 在实际生产环境中，你可能需要更复杂的 CPU 使用率计算
			metrics.MQTTCPUUsage.Set(float64(runtime.NumGoroutine()) / 100.0)
		}
	}()
}

func init() {
	rootCmd.AddCommand(connCmd)
	connCmd.Flags().IntVar(&metricsPort, "metrics-port", 2112, "Port to expose Prometheus metrics")
	connCmd.Flags().Int("keep-time", 0, "Time to keep connections alive after all connections are established (in seconds). 0: don't keep, -1: keep forever. Example: --keep-time=60")
}
