# MQTT Benchmark Tool

A powerful, cross-platform, and highly performant benchmark tool for MQTT brokers written in Go. It allows you to test various aspects of MQTT broker performance including connection handling, publishing, and subscribing. The tool provides detailed metrics through Prometheus integration, supports multiple connection and message patterns, and can be easily distributed on Kubernetes.

### Features

- Multiple client connections simulation
- Configurable QoS levels
- Message rate control
- Connection rate limiting
- Customizable payload sizes
- Multiple topic support with topic template
- Latency measurements
- Prometheus metrics integration for easy visualization
- Detailed logging
- pprof support for profiling
- Docker support for easy deployment
- Kubernetes support for distributed load testing

## Installation

```bash
go install github.com/zzjcool/mqtt-benchmark@latest
```

## Usage

The tool provides three main commands:
- `conn`: For testing connection handling
- `pub`: For publishing messages
- `sub`: For subscribing to topics

### Global Flags

```bash
  -S, --servers strings        MQTT broker addresses (default [127.0.0.1:1883])
  -u, --user string           MQTT username
  -P, --pass string           MQTT password
  -c, --clientNum uint16      Number of MQTT clients to create (default 100)
      --log-level string      Log level (debug, info, warn, error) (default "info")
  -n, --client-prefix string  Client ID prefix (default "mqtt-benchmark")
  -L, --clean                 Use clean session (default true)
      --keepalive int         Keepalive interval in seconds (default 60)
      --num-retry-connect int Number of connection retry attempts
  -R, --connrate int         Connection rate limit per second
      --metrics-port int     Prometheus metrics port (default 2112)
      --pprof-port int      pprof port (0 to disable)
```

### Topic Templates

The tool supports two special placeholders in topic patterns:
- `%d`: Replaced with the topic number (0 to topic-num-1)
- `%i`: Replaced with the client index (0 to clientNum-1)

This allows for flexible topic patterns that can distribute load across different topics and clients.

Examples:
```bash
# Using topic number: creates topics test/0, test/1, test/2 for each client
mqtt-benchmark pub -t "test/%d" -N 3 -C 100

# Using client index: each client publishes to its own topic (client0/data, client1/data, etc)
mqtt-benchmark pub -t "client%i/data" -c 5 -C 100

# Combining both: each client publishes to multiple numbered topics
mqtt-benchmark pub -t "client%i/data/%d" -c 2 -N 3 -C 100
# This creates:
# Client 0: client0/data/0, client0/data/1, client0/data/2
# Client 1: client1/data/0, client1/data/1, client1/data/2
```

### Connection Command (conn)

```bash
mqtt-benchmark conn [flags]

Flags:
      --keep-time int      Time to keep connections alive (seconds)
                          0: don't keep, -1: keep forever (default 0)
```

Example:
```bash
# Connect 1000 clients at rate of 100 connections per second
mqtt-benchmark conn -c 1000 -R 100

# Connect 500 clients and keep connections for 5 minutes
mqtt-benchmark conn -c 500 --keep-time 300

# Connect to multiple brokers with authentication
mqtt-benchmark conn -S broker1:1883 -S broker2:1883 -u user1 -P pass1 -c 100

# Connect with custom client IDs and clean session disabled
mqtt-benchmark conn -n "custom-client" -L=false -c 50
```

### Publisher Command (pub)

```bash
mqtt-benchmark pub [flags]

Flags:
  -t, --topic string          Topic to publish to (required)
  -N, --topic-num int        Number of topics per client (default 1)
  -p, --payload string       Message payload
  -s, --payload-size int     Size of generated payload in bytes (default 100)
  -q, --qos int             QoS level (0, 1, or 2) (default 0)
  -C, --count int           Number of messages to publish (default 1)
  -i, --interval int        Interval between messages in milliseconds
  -r, --rate int           Message rate limit per second
      --timeout duration    Operation timeout (default 5s)
      --with-timestamp     Add timestamp to message payload
```

Example:
```bash
# Publish 1000 messages of 100 bytes to topic "test" with QoS 1
mqtt-benchmark pub -t test -q 1 -C 1000 -s 100

# Publish to multiple topics per client
mqtt-benchmark pub -t "test/%d" -N 3 -C 100

# Each client publishes to its own set of topics
mqtt-benchmark pub -t "client%i/sensor/%d" -c 5 -N 2 -C 100
```

### Subscriber Command (sub)

```bash
mqtt-benchmark sub [flags]

Flags:
  -t, --topic string          Topic to subscribe to (required)
  -N, --topic-num int        Number of topics per client (default 1)
  -q, --qos int             QoS level (0, 1, or 2) (default 0)
      --timeout duration    Operation timeout (default 5s)
      --parse-timestamp    Parse timestamp from message payload
      --keep-time int      Time to keep connections alive after subscription
```

Example:
```bash
# Subscribe to topic "test" with QoS 1
mqtt-benchmark sub -t test -q 1

# Subscribe to multiple topics per client
mqtt-benchmark sub -t "test/%d" -N 3

# Each client subscribes to its own set of topics
mqtt-benchmark sub -t "client%i/sensor/%d" -c 5 -N 2
```

## Metrics

The tool exposes Prometheus metrics on the configured metrics port (default: 2112). Available metrics include:

- Connection statistics
- Message rates
- Latency measurements
- Error counts
- QoS distribution

Access metrics at: `http://localhost:2112/metrics`

## Profiling

When pprof port is enabled, you can profile the application using Go's pprof tools:

```bash
# Enable pprof on port 6060
mqtt-benchmark pub -t test --pprof-port 6060

# Access pprof endpoints
go tool pprof http://localhost:6060/debug/pprof/heap
go tool pprof http://localhost:6060/debug/pprof/profile
```

## Local Monitoring Setup

### Using Docker Compose

To start the monitoring stack locally:

```bash
docker compose up -d
```

Access the monitoring dashboards:
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (default credentials: admin/admin)

### Using Kubernetes

1. Create monitoring namespace and deploy Prometheus:

```bash
kubectl create namespace monitoring
kubectl apply -f k8s-prometheus.yaml
```

2. Deploy Grafana:

```bash
kubectl apply -f k8s-grafana.yaml
```

3. Import Grafana Dashboard:
   - Access Grafana web interface
   - Click on '+' icon and select 'Import'
   - Click 'Upload JSON file'
   - Select the dashboard JSON file from `grafana/dashboards/*-dashboard.json`
   - Select the Prometheus data source
   - Click 'Import'

4. Access the monitoring dashboards:
   - Get Prometheus URL:
     ```bash
     kubectl get svc -n monitoring prometheus-service
     ```
   - Get Grafana URL:
     ```bash
     kubectl get svc -n monitoring grafana-service
     ```

## Running Benchmark in Kubernetes

1. Create benchmark deployment:

```bash
kubectl apply -f deployment.yaml
```

2. Scale the benchmark:

```bash
kubectl scale deployment mqtt-benchmark --replicas=3
```

3. View benchmark logs:

```bash
kubectl logs -l app=mqtt-benchmark -f
```

4. Monitor metrics:
   - Open Grafana dashboard
   - Import the MQTT Benchmark dashboard (ID: provided in grafana/dashboards)
   - View real-time metrics including:
     - Connection success/failure rates
     - Message throughput
     - Latency statistics
     - Client counts

## License

This project is licensed under the MIT License - see the LICENSE file for details.
