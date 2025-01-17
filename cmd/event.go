package cmd

import (
	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/mqtt"
)

var FlagwaitTimeSeconds = "wait-time-seconds"

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Run MQTT event test",
	Long:  `Test MQTT client connection and disconnection events`,
	RunE: func(cmd *cobra.Command, args []string) error {
		options := fillMqttOptions(cmd)

		waitTimeSeconds, _ := cmd.Flags().GetInt(FlagwaitTimeSeconds)
		executor := mqtt.NewEventExecutor(options, waitTimeSeconds)
		return executor.RunEventTest()
	},
}

func init() {
	rootCmd.AddCommand(eventCmd)
	eventCmd.Flags().Int(FlagwaitTimeSeconds, 5, "Timeout for subscribe operations in seconds")

}
