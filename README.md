# grpcd

**Reference server implementation of the gRPC Method Discovery (grpcd) service**

This is the canonical Go implementation of the grpcd service as defined in
[grpcd-protos](https://github.com/sonic-original-software/grpcd-protos).

For details on what gRPC Method Discovery is, its architecture, and use cases,
see the
[grpcd-protos documentation](https://github.com/sonic-original-software/grpcd-protos).

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Implementation](#implementation)
- [Storage Backends](#storage-backends)
- [Development](#development)

## Overview

This implementation demonstrates production-ready patterns for building
stateless, horizontally-scalable gRPC services with pluggable storage backends.

Production-ready container images are published to
[`ghcr.io/sonic-original-software/grpcd`](https://github.com/sonic-original-software/grpcd/pkgs/container/grpcd)
for easy deployment.

### Implementation Features

- **Production Ready**: Pre-built container images, OpenTelemetry metrics,
  structured logging, health checks
- **Pluggable Storage**: Abstract storage interface with Redis and in-memory
  implementations
- **Peer Context Security**: Extracts real client addresses from gRPC peer
  context to prevent spoofing
- **Docker Native**: Multi-stage builds, scratch-based images, Docker Compose
  support

## Quick Start

### Using the Pre-built Container Image

Ready-to-use container images are available at
[]`ghcr.io/sonic-original-software/grpcd`](https://github.com/sonic-original-software/grpcd/pkgs/container/grpcd).

```bash
# Run with in-memory storage (development/testing)
docker run -p 5000:5000 ghcr.io/sonic-original-software/grpcd:latest

# Run with Redis (production)
docker run -p 5000:5000 \
  -e STORAGE_BACKEND=redis \
  -e STORAGE_ADDRESS=redis:6379 \
  ghcr.io/sonic-original-software/grpcd:latest
```

This is the recommended way to deploy grpcd in production environments.

### Using Docker Compose

The easiest way to run grpcd locally with Redis:

```bash
docker compose up
```

This starts:

- Redis on port 6379
- grpcd on port 53001 (mapped to internal port 5000)

### Building from Source

Requirements:

- Go 1.25.3 or later
- Redis (optional, for persistent storage)

```bash
# Clone the repository
git clone https://github.com/sonic-original-software/grpcd
cd grpcd

# Download dependencies
go mod download

# Build
go build -o grpcd .

# Run with in-memory storage
./grpcd

# Run with Redis storage
STORAGE_BACKEND=redis STORAGE_ADDRESS=localhost:6379 ./grpcd
```

## Configuration

grpcd is configured entirely through environment variables.

### Server Configuration

| Variable              | Description                     | Default |
| --------------------- | ------------------------------- | ------- |
| `GRPC_SERVER_ADDRESS` | Address to bind the gRPC server | `:5000` |

### Storage Configuration

| Variable          | Description                                           | Default | Required             |
| ----------------- | ----------------------------------------------------- | ------- | -------------------- |
| `STORAGE_BACKEND` | Storage backend type (`redis` or empty for in-memory) | -       | No                   |
| `STORAGE_ADDRESS` | Storage backend address (format depends on backend)   | -       | Yes (if using Redis) |

### Registration Configuration

| Variable                   | Description                                         | Default |
| -------------------------- | --------------------------------------------------- | ------- |
| `REGISTRATION_TTL_MINUTES` | How long registrations remain valid without refresh | `10`    |

### Example Configurations

**Development (in-memory storage)**:

```bash
export GRPC_SERVER_ADDRESS=:5000
# No STORAGE_BACKEND set = in-memory storage
```

**Production (Redis storage)**:

```bash
export GRPC_SERVER_ADDRESS=:5000
export STORAGE_BACKEND=redis
export STORAGE_ADDRESS=redis.prod.example.com:6379
export REGISTRATION_TTL_MINUTES=10
```

## Implementation

This server implements the
[GRPCDService](https://github.com/sonic-original-software/grpcd-protos) defined
in grpcd-protos. For the complete API specification and protobuf definitions,
refer to the
[grpcd-protos repository](https://github.com/sonic-original-software/grpcd-protos).

### RPC Implementations

| RPC          | Implementation                      | Notes                                                 |
| ------------ | ----------------------------------- | ----------------------------------------------------- |
| `Register`   | `internal/service/register.go:31`   | Extracts real address from peer context for security  |
| `Discover`   | `internal/service/discover.go:26`   | Returns `NOT_FOUND` if method is not registered       |
| `Deregister` | `internal/service/deregister.go:20` | Removes all methods for the calling service's address |

The server also implements standard diagnostic services defined in
`grpc-protos`:

- **Info Service**: `internal/service/get_info.go`
- **Diagnostics Service**: `internal/service/get_diagnostics.go`

### Security Implementation

This implementation uses gRPC peer context to extract the real client address
for `Register` and `Deregister` operations. The peer context contains TCP
connection metadata that cannot be spoofed, preventing address hijacking.

See `internal/service/register.go:43` and `internal/service/deregister.go:26`
for implementation details.

## Storage Backends

grpcd uses an abstract storage interface (`internal/storage/store.go:34`) that
supports multiple backends.

### Interface

```go
type Store interface {
    SetMethodAddress(ctx, method, address string, ttl time.Duration) error
    GetMethodAddress(ctx context.Context, method string) (string, error)
    DeleteMethodAddress(ctx context.Context, method string) error
    GetMethodsByAddress(ctx context.Context, address string) ([]string, error)
    DeleteMethodsByAddress(ctx context.Context, address string) error
    Ping(ctx context.Context) error
    Name() string
    Address() string
}
```

### Available Backends

#### In-Memory (Mock)

**Use Case**: Development, testing, single-instance deployments

**Characteristics**:

- No external dependencies
- Data lost on restart
- Not suitable for production multi-instance deployments
- Automatically selected when `STORAGE_BACKEND` is not set

**Implementation**: `internal/storage/mock/`

#### Redis

**Use Case**: Production deployments

**Characteristics**:

- Persistent storage with optional persistence to disk
- Native TTL support for automatic cleanup
- Horizontal scaling support (all grpcd instances share state)
- High availability with Redis Sentinel or Redis Cluster

**Configuration**:

```bash
STORAGE_BACKEND=redis
STORAGE_ADDRESS=redis-host:6379
```

**Implementation**: `internal/storage/redis/`

### Adding a New Backend

1. Implement the `Store` interface
2. Add resolver logic in `internal/storage/resolver/resolve.go`

## Development

### Project Structure

```
grpcd/
├── main.go                      # Entry point, server setup
├── internal/
│   ├── service/                 # RPC method implementations
│   │   ├── register.go          # Register RPC
│   │   ├── discover.go          # Discover RPC
│   │   ├── deregister.go        # Deregister RPC
│   │   └── server.go            # Server type and constructor
│   ├── storage/                 # Storage abstraction
│   │   ├── store.go             # Store interface
│   │   ├── resolver/            # Backend resolution logic
│   │   ├── mock/                # In-memory implementation
│   │   └── redis/               # Redis implementation
│   └── validate/                # Request validation
├── Dockerfile                   # Multi-stage Docker build
└── compose.yaml                 # Local development with Redis
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/service/...

# Run fuzz tests
go test -fuzz=FuzzRegister ./internal/service
go test -fuzz=FuzzDiscover ./internal/service
```

### Building

```bash
# Standard build
go build -o grpcd .

# Optimized build (same as Dockerfile)
CGO_ENABLED=0 go build -ldflags="-w -s" -o grpcd .
```

### Dependencies

Major dependencies:

- `grpcd-protos`: Service definition and generated code
- `grpcd-go`: Client library for registering/discovering methods
- `grpc-foundation`: Common gRPC server utilities and error handling
- `grpc-protos`: Standard service interfaces (Info, Diagnostics)
- `go-redis/v9`: Redis client
- `google.golang.org/grpc`: gRPC framework
- `go.opentelemetry.io/otel`: Observability instrumentation

See `go.mod` for complete list.

### Code Quality

The codebase includes:

- Unit tests for all RPC methods
- Fuzz tests for request validation
- Structured logging with contextual information
- OpenTelemetry metrics for monitoring
- Input validation and error handling

## License

See LICENSE file for details.

## Related Projects

- [grpcd-protos](https://github.com/sonic-original-software/grpcd-protos):
  Protocol buffer definitions and service specification
- [grpcd-go](https://git.sonicoriginal.software/grpcd-go): Go client library for
  gRPC Method Discovery

## Contributing

Contributions welcome! This is the reference implementation, so changes should
align with the grpcd specification in grpcd-protos.
