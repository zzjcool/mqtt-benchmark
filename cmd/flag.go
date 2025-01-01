package cmd

import "time"

const (
	FlagTopic          = "topic"
	FlagTopicNum       = "topic-num"
	FlagPayload        = "payload"
	FlagPayloadSize    = "payload-size"
	FlagQoS            = "qos"
	FlagCount          = "count"
	FlagInterval       = "interval"
	FlagRate           = "rate"
	FlagTimeout        = "timeout"
	FlagWithTimestamp  = "with-timestamp"
	FlagParseTimestamp = "parse-timestamp"
	FlagWaitForClients = "wait-for-clients"
)

func init() {
	rootCmd.PersistentFlags().StringP(FlagTopic, "t", "", "Topic to publish/subscribe to")
	rootCmd.PersistentFlags().IntP(FlagTopicNum, "N", 1, "Number of topics per client")
	rootCmd.PersistentFlags().StringP(FlagPayload, "p", "", "Message payload")
	rootCmd.PersistentFlags().IntP(FlagPayloadSize, "s", 100, "Size of the message payload to generate")
	rootCmd.PersistentFlags().IntP(FlagQoS, "q", 0, "QoS level (0, 1, or 2)")
	rootCmd.PersistentFlags().IntP(FlagInterval, "i", 0, "Interval between messages in milliseconds")
	rootCmd.PersistentFlags().IntP(FlagRate, "r", 0, "Rate limit in messages per second")
	rootCmd.PersistentFlags().DurationP(FlagTimeout, "", 5*time.Second, "Timeout for operations")
	rootCmd.PersistentFlags().BoolP(FlagWithTimestamp, "", false, "Add timestamp to message payload")
	rootCmd.PersistentFlags().BoolP(FlagParseTimestamp, "", false, "Parse timestamp from message payload")
	rootCmd.PersistentFlags().BoolP(FlagWaitForClients, "w", false, "Wait for other clients to be ready before starting")

	rootCmd.MarkPersistentFlagRequired(FlagTopic)
}
