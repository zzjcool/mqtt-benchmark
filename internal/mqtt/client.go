package mqtt

import mqtt "github.com/eclipse/paho.mqtt.golang"

type NewClientFunc func(o *mqtt.ClientOptions) mqtt.Client
