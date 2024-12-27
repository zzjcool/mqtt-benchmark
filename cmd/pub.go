/*
Copyright 2024 NAME HERE <EMAIL ADDRESS>
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
		topicNum, _ := cmd.Flags().GetInt(FlagTopicNum)
		payload, _ := cmd.Flags().GetString(FlagPayload)
		payloadSize, _ := cmd.Flags().GetInt(FlagPayloadSize)
		qos, _ := cmd.Flags().GetInt(FlagQoS)
		count, _ := cmd.Flags().GetInt(FlagCount)
		rate, _ := cmd.Flags().GetFloat64(FlagRate)
		timeout, _ := cmd.Flags().GetInt(FlagTimeout)
		withTimestamp, _ := cmd.Flags().GetBool(FlagWithTimestamp)
		inflight, _ := cmd.Flags().GetInt(FlagInflight)

		// Validate topic template if topic-num is set
		if err := internalmqtt.ValidateTopicTemplate(topic, topicNum); err != nil {
			log.Error("Invalid topic template", zap.Error(err))
			os.Exit(1)
		}

		// Get MQTT options
		options := fillMqttOptions(cmd)

		// Create publisher
		publisher := internalmqtt.NewPublisher(options, topic, topicNum, options.ClientIndex, payload, payloadSize, qos, count, rate)
		if timeout > 0 {
			publisher.SetTimeout(time.Duration(timeout) * time.Second)
		}
		publisher.SetWithTimestamp(withTimestamp)
		publisher.SetInflight(inflight)

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
	pubCmd.Flags().Int(FlagTopicNum, 1, "Number of topics to publish to")
	pubCmd.Flags().String(FlagPayload, "", "Fixed payload to publish")
	pubCmd.Flags().Int(FlagPayloadSize, 100, "Size of random payload in bytes")
	pubCmd.Flags().Int(FlagQoS, 0, "QoS level (0, 1, or 2)")
	pubCmd.Flags().Int(FlagCount, 0, "Number of messages to publish, default 0 (infinite)")
	pubCmd.Flags().Float64(FlagRate, 1.0, "Messages per second per client")
	pubCmd.Flags().Int(FlagTimeout, 5, "Timeout for publish operations in seconds")
	pubCmd.Flags().Bool(FlagWithTimestamp, false, "Add timestamp to the beginning of payload")
	pubCmd.Flags().Int(FlagInflight, 1, "Maximum inflight messages for QoS 1 and 2, value 0 for 'infinity'")
}
