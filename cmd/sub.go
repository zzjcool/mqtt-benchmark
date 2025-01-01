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
		topicNum, _ := cmd.Flags().GetInt(FlagTopicNum)
		qos, _ := cmd.Flags().GetInt(FlagQoS)
		timeout, _ := cmd.Flags().GetInt(FlagTimeout)
		parseTimestamp, _ := cmd.Flags().GetBool(FlagParseTimestamp)

		// Validate topic template if topic-num is set
		if err := internalmqtt.ValidateTopicTemplate(topic, topicNum); err != nil {
			log.Error("Invalid topic template", zap.Error(err))
			os.Exit(1)
		}

		// Get MQTT options
		options := fillMqttOptions(cmd)

		// Create subscriber
		subscriber := internalmqtt.NewSubscriber(options, topic, topicNum, options.ClientIndex, qos)
		if timeout > 0 {
			subscriber.SetTimeout(time.Duration(timeout) * time.Second)
		}
		subscriber.SetParseTimestamp(parseTimestamp)

		// Run subscription test
		if err := subscriber.RunSubscribe(); err != nil {
			log.Error("Failed to run subscription", zap.Error(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(subCmd)

	// Add sub-specific flags
	subCmd.PersistentFlags().StringP(FlagTopic, "t", "", "Topic to subscribe to")
	subCmd.MarkPersistentFlagRequired(FlagTopic)
	subCmd.PersistentFlags().IntP(FlagTopicNum, "N", 1, "Number of topics to subscribe to")
	subCmd.PersistentFlags().IntP(FlagQoS, "q", 0, "QoS level (0, 1, or 2)")
	subCmd.Flags().Int(FlagTimeout, 5, "Timeout for subscribe operations in seconds")
	subCmd.Flags().Bool(FlagParseTimestamp, false, "Parse timestamp from the beginning of payload")
}
