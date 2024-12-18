package mqtt

type Options struct {
	Servers          []string
	User             string
	Password         string
	KeepAliveSeconds int
	ClientNum        uint16
	ClientPrefix     string
	ConnRate         int
	QoS              int
	Retain           bool
	AutoReconnect    bool
	CleanSession     bool
}
