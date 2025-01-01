package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	internalmqtt "github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"go.uber.org/zap"
)

// connCmd represents the conn command
var connCmd = &cobra.Command{
	Use:   "conn",
	Short: "Connect multiple MQTT clients to broker(s)",
	Long: `Connect multiple MQTT clients to one or more MQTT brokers with specified parameters.
This command allows you to test broker connection handling with various parameters like
connection rate, number of clients, and authentication settings.`,
	Run: func(cmd *cobra.Command, args []string) {
		log := logger.GetLogger()

		// 获取保持连接时间
		keepTime, _ := cmd.Flags().GetInt("keep-time")

		// 获取 MQTT 选项
		options := fillMqttOptions(cmd)

		// 创建连接管理器
		connManager := internalmqtt.NewConnectionManager(options, keepTime)

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

		connManager.DisconnectAll()
	},
}

func init() {
	rootCmd.AddCommand(connCmd)
	connCmd.Flags().Int("keep-time", 0, "Time to keep connections alive after all connections are established (in seconds). 0: don't keep, -1: keep forever. Example: --keep-time=60")
}
