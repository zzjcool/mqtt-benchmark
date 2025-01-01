# MQTT Benchmark Tool

A powerful, cross-platform, and highly performant benchmark tool for MQTT brokers written in Go. It allows you to test various aspects of MQTT broker performance including connection handling, publishing, and subscribing. The tool provides detailed metrics through Prometheus integration, supports multiple connection and message patterns, and can be easily distributed on Kubernetes.

## Development

For development guidelines, coding standards, and contribution process, please refer to [CONTRIBUTING.md](CONTRIBUTING.md).

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

### Using Go Install

```bash
go install github.com/zzjcool/mqtt-benchmark@latest
```

### Download Binary

You can download pre-built binaries for your platform from the [releases page](https://github.com/zzjcool/mqtt-benchmark/releases).

### Using Docker

You can run mqtt-benchmark using the official Docker image:

```bash
docker pull zzjcool/mqtt-benchmark
docker run zzjcool/mqtt-benchmark --help

# Example: Running connection test
docker run zzjcool/mqtt-benchmark conn -S host.docker.internal:1883 -c 100

# Example: Publishing messages
docker run zzjcool/mqtt-benchmark pub -S host.docker.internal:1883 -t test -C 100
```

Note: When running in Docker and connecting to a broker on your host machine, use `host.docker.internal` instead of `localhost`.

## Usage

The tool provides three main commands:
- `conn`: For testing connection handling
- `pub`: For publishing messages
- `sub`: For subscribing to topics

### Global Flags

```bash
      --ca-cert-file string       Path to CA certificate file
      --ca-key-file string        Path to CA private key file for dynamic certificate generation
  -L, --clean                     Clean session (default true)
      --client-cert-file string   Path to client certificate file
      --client-key-file string    Path to client key file
  -n, --client-prefix string      Client ID prefix (default "mqtt-benchmark")
  -c, --clientNum uint32         Number of MQTT clients (default 100)
      --connect-timeout int       Connection timeout in seconds (default 10)
  -R, --connrate int             Connection rate limit per second
      --keepalive int            Keepalive interval in seconds (default 60)
      --log-level string         Log level (debug, info, warn, error) (default "info")
      --metrics-port int         Port to expose Prometheus metrics (default 2112)
      --num-retry-connect int    Number of times to retry connecting
  -p, --pass string              MQTT broker password
      --pprof-port int          pprof port (0 to disable)
  -s, --servers stringArray      MQTT broker addresses (default [127.0.0.1:1883])
      --skip-verify             Skip server certificate verification
  -u, --user string             MQTT broker username
  -w, --wait-for-clients        Wait for other clients to be ready before starting
      --write-timeout int       Write timeout in seconds (default 5)
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

The `conn` command is used to test broker connection handling capabilities with various parameters like connection rate, number of clients, and authentication settings.

```bash
mqtt-benchmark conn [flags]

# All global flags are also available for this command
Flags:
      --keep-time int   Time to keep connections alive after all connections are established (in seconds)
                       0: don't keep, -1: keep forever
                       Example: --keep-time=60
```

Example:
```bash
# Connect 1000 clients at rate of 100 connections per second
mqtt-benchmark conn -c 1000 -R 100

# Connect with TLS using certificates and keep connections for 5 minutes
mqtt-benchmark conn --ca-cert-file ca.crt --client-cert-file client.crt --client-key-file client.key --keep-time 300

# Connect with authentication, custom timeouts and wait for all clients
mqtt-benchmark conn -u user1 -p pass1 --connect-timeout 15 --write-timeout 10 -w
```

### Publisher Command (pub)

The `pub` command is used to test broker publishing performance with various parameters like message size, QoS level, publishing rate, and number of messages.

```bash
mqtt-benchmark pub [flags]

# All global flags are also available for this command
Flags:
      --count int          Number of messages to publish, default 0 (infinite)
      --inflight int       Maximum inflight messages for QoS 1 and 2, value 0 for 'infinity'
      --payload string     Fixed payload to publish
  -S, --payload-size int   Size of random payload in bytes (default 100)
  -q, --qos int           QoS level (0, 1, or 2)
      --rate float        Messages per second per client (default 1)
      --timeout int       Timeout for publish operations in seconds (default 5)
  -t, --topic string      Topic to publish to
      --topic-num int     Number of topics to publish to (default 1)
      --with-timestamp   Add timestamp to the beginning of payload
```

Example:
```bash
# Publish infinite messages with QoS 1 and 100 byte payload at rate of 100 msg/s
mqtt-benchmark pub -t test -q 1 -S 100 --rate 100

# Publish to multiple topics with custom payload and inflight limit
mqtt-benchmark pub -t sensor/data --topic-num 5 --payload "test data" --inflight 100

# Publish with TLS and timestamp in payload
mqtt-benchmark pub -t events --ca-cert-file ca.crt --with-timestamp --timeout 10
```

### Subscriber Command (sub)

The `sub` command is used to test broker subscription performance with various parameters like QoS level and topic filters.

```bash
mqtt-benchmark sub [flags]

# All global flags are also available for this command
Flags:
      --parse-timestamp   Parse timestamp from the beginning of payload
  -q, --qos int          QoS level (0, 1, or 2)
      --timeout int      Timeout for subscribe operations in seconds (default 5)
  -t, --topic string     Topic to subscribe to
  -N, --topic-num int    Number of topics to subscribe to (default 1)
```

Example:
```bash
# Subscribe to multiple topics with QoS 1
mqtt-benchmark sub -t "test/#" -q 1 -N 5

# Subscribe with timestamp parsing and custom timeout
mqtt-benchmark sub -t "sensor/+" --parse-timestamp --timeout 30

# Subscribe with TLS
mqtt-benchmark sub -t events --ca-cert-file ca.crt --client-cert-file client.crt
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
