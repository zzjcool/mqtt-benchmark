/*
Copyright 2024 NAME HERE EMAIL ADDRESS
*/
package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	internalmqtt "github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"go.uber.org/zap"
)

// subCmd represents the sub command
var subCmd = &cobra.Command{
	Use:   "sub",
	Short: "Subscribe to messages from MQTT broker(s)",
	Long: `Subscribe to messages from MQTT broker(s) with specified parameters.
This command allows you to test broker subscription performance with various parameters like
QoS level and topic filters.`,
	Run: func(cmd *cobra.Command, args []string) {
		log := logger.GetLogger()

		// Get subscription parameters
		topic, _ := cmd.Flags().GetString(FlagTopic)
		qos, _ := cmd.Flags().GetInt(FlagQoS)
		timeout, _ := cmd.Flags().GetInt(FlagTimeout)
		keepTime, _ := cmd.Flags().GetInt("keep-time")
		parseTimestamp, _ := cmd.Flags().GetBool(FlagParseTimestamp)

		// Get MQTT options
		options := fillMqttOptions(cmd)

		// Create subscriber
		subscriber := internalmqtt.NewSubscriber(options, topic, qos)
		if timeout > 0 {
			subscriber.SetTimeout(time.Duration(timeout) * time.Second)
		}
		subscriber.SetParseTimestamp(parseTimestamp)

		// Run subscription test
		if err := subscriber.RunSubscribe(); err != nil {
			log.Error("Failed to run subscription", zap.Error(err))
			os.Exit(1)
		}

		// Keep connections if specified
		if keepTime != 0 {
			connManager := internalmqtt.NewConnectionManager(options, keepTime)
			if err := connManager.KeepConnections(); err != nil {
				log.Error("Failed to keep connections", zap.Error(err))
				os.Exit(1)
			}
		}
	},
}

const (
	FlagParseTimestamp = "parse-timestamp"
)

func init() {
	rootCmd.AddCommand(subCmd)

	// Add sub-specific flags
	subCmd.Flags().String(FlagTopic, "test", "Topic to subscribe to")
	subCmd.Flags().Int(FlagQoS, 0, "QoS level (0, 1, or 2)")
	subCmd.Flags().Int(FlagTimeout, 5, "Timeout for subscribe operations in seconds")
	subCmd.Flags().Int("keep-time", 0, "Time to keep connections alive after subscription (0 means no keep-alive)")
	subCmd.Flags().Bool(FlagParseTimestamp, false, "Parse timestamp from the beginning of payload")
}
