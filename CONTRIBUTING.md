# Contributing Guide

## Development Standards

### Code Structure
- `cmd/`: Contains the main applications
- `internal/`: Internal packages
- `certs/`: Certificate related code
- `grafana/`: Grafana configurations
- `k8s-*.yaml`: Kubernetes deployment configurations

### Development Environment Setup
1. Required Tools:
   - Go 1.23 or higher
   - Docker
   - Kubernetes (optional, for deployment)
   - VSCode with Go extension (recommended)

2. Initial Setup:
```bash
git clone <repository-url>
cd mqtt-benchmark
go mod download
```

### Code Style Guidelines
1. Go Code Style
   - Follow the official Go style guide
   - Use `gofmt` for code formatting
   - Maximum line length: 120 characters
   - Use meaningful variable and function names
   - Add comments for public functions and complex logic

2. Code Organization
   - Place interfaces in separate files
   - Group related functionality in packages
   - Keep files focused and not too large (< 500 lines recommended)
   - All mock code for MQTT clients in `internal/mqtt` package should be placed in `mqtt_test.go`

### General Development Guidelines
1. Language Usage
   - Input and thinking process should be in English
   - Output should match the language of input
   - All code comments must be in English

2. Testing and Command Execution
   - Unit tests and information viewing commands should be executed automatically
   - All test and long-running commands should include timeout parameters
   - Default timeouts should be reasonable for the operation

### Testing Requirements
1. Unit Tests
   - All new code must include unit tests
   - Test coverage should be maintained at minimum 80%
   - Use table-driven tests when appropriate
   - Run tests with race detector: `go test -race ./...`

2. Integration Tests
   - Write integration tests for MQTT connections
   - Test with different QoS levels
   - Include timeout in all tests

### Documentation
1. Code Documentation
   - Document all exported functions and types
   - Include examples in documentation where helpful
   - Keep documentation up to date with code changes

2. Project Documentation
   - Update README.md for major changes
   - Document configuration options
   - Include deployment instructions
