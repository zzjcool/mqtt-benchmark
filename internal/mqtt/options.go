package mqtt

type Options struct {
	Servers              []string
	User                 string
	Password             string
	KeepAliveSeconds     int
	ClientNum            uint16
	ClientPrefix         string
	ConnRate             int
	QoS                  int
	Retain               bool
	ClientIndex          int
	AutoReconnect        bool
	CleanSession         bool
	ConnectRetryInterval int  // Seconds between connection retries
	ConnectTimeout       int  // Connection timeout in seconds
	ConnectRetry         bool // Whether to retry connection
}
