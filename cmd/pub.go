/*
Copyright 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/zzjcool/mqtt-benchmark/internal/logger"
	internalmqtt "github.com/zzjcool/mqtt-benchmark/internal/mqtt"
	"go.uber.org/zap"
)

const (
	FlagTopic       = "topic"
	FlagPayload     = "payload"
	FlagPayloadSize = "payload-size"
	FlagQoS         = "qos"
	FlagCount       = "count"
	FlagInterval    = "interval"
)

// pubCmd represents the pub command
var pubCmd = &cobra.Command{
	Use:   "pub",
	Short: "Publish messages to MQTT broker(s)",
	Long: `Publish messages to MQTT broker(s) with specified parameters.
This command allows you to test broker publishing performance with various parameters like
message size, QoS level, publishing rate, and number of messages.`,
	Run: func(cmd *cobra.Command, args []string) {
		log := logger.GetLogger()

		// Get publish parameters
		topic, _ := cmd.Flags().GetString(FlagTopic)
		payload, _ := cmd.Flags().GetString(FlagPayload)
		payloadSize, _ := cmd.Flags().GetInt(FlagPayloadSize)
		qos, _ := cmd.Flags().GetInt(FlagQoS)
		count, _ := cmd.Flags().GetInt(FlagCount)
		interval, _ := cmd.Flags().GetInt(FlagInterval)

		// Get MQTT options
		options := fillMqttOptions(cmd)

		// Create publisher
		publisher := internalmqtt.NewPublisher(options, topic, payload, payloadSize, qos, count, interval)

		// Run publishing test
		if err := publisher.RunPublish(); err != nil {
			log.Error("Failed to run publishing", zap.Error(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(pubCmd)

	// Add pub-specific flags
	pubCmd.Flags().String(FlagTopic, "test", "Topic to publish to")
	pubCmd.Flags().String(FlagPayload, "", "Fixed payload to publish")
	pubCmd.Flags().Int(FlagPayloadSize, 100, "Size of random payload in bytes")
	pubCmd.Flags().Int(FlagQoS, 0, "QoS level (0, 1, or 2)")
	pubCmd.Flags().Int(FlagCount, 1000, "Number of messages to publish")
	pubCmd.Flags().Int(FlagInterval, 1000, "Interval between messages in milliseconds")
}
