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

| Variable                  | Description                                                  | Default |
| ------------------------- | ------------------------------------------------------------ | ------- |
| `GRPC_SERVER_ADDRESS`     | Address to bind the gRPC server                              | `:5000` |
| `GRPC_MAX_CONNECTION_AGE` | Age at which the server sends a GOAWAY. `0` is no limit.     | `10m`   |

Set `GRPC_MAX_CONNECTION_AGE=0`.

A registration lives as long as the stream holding it, and a GOAWAY ends that
stream whether or not it is active. At the ten minute default, every
registration in the mesh is torn down and rebuilt on that timer. It works, and
it looks healthy, while costing a reconnect per service per interval.

grpcd reports the value in effect at startup.

### Storage Configuration

| Variable          | Description                                           | Default | Required             |
| ----------------- | ----------------------------------------------------- | ------- | -------------------- |
| `STORAGE_BACKEND` | Storage backend type (`redis` or empty for in-memory) | -       | No                   |
| `STORAGE_ADDRESS` | Storage backend address (format depends on backend)   | -       | Yes (if using Redis) |

A registration lives as long as the stream that made it, so nothing configures
how long one lasts.

### Health

The health service answers for two entries. `""` says the process is alive.
`grpcd.GRPCDService` says whether the store can be reached: it reports
`NOT_SERVING` from the first operation that fails against the store until the
store's subscription comes back, and `SERVING` otherwise.

A balancer or orchestrator probe that should route around an instance that
has lost its store asks for `grpcd.GRPCDService`. A probe that asks for `""`
sees only whether the process is up.

While the store is lost the instance keeps serving: handlers hold what they
have and wait for the store rather than failing, and when it returns every
held registration writes its rows again. Pulling the instance from rotation,
or restarting it, is whoever probes it deciding to; nothing here does either.

### Example Configurations

**Development (in-memory storage)**:

```bash
export GRPC_SERVER_ADDRESS=:5000
export GRPC_MAX_CONNECTION_AGE=0
# No STORAGE_BACKEND set = in-memory storage
```

**Production (Redis storage)**:

```bash
export GRPC_SERVER_ADDRESS=:5000
export GRPC_MAX_CONNECTION_AGE=0
export STORAGE_BACKEND=redis
export STORAGE_ADDRESS=redis.prod.example.com:6379
```

## Implementation

This server implements the
[GRPCDService](https://github.com/sonic-original-software/grpcd-protos) defined
in grpcd-protos. For the complete API specification and protobuf definitions,
refer to the
[grpcd-protos repository](https://github.com/sonic-original-software/grpcd-protos).

### RPC Implementations

| RPC        | Implementation                 | Notes                                                                 |
| ---------- | ------------------------------ | --------------------------------------------------------------------- |
| `Register` | `internal/service/register.go` | Holds the stream; writes the rows on open and removes them on its end |
| `Discover` | `internal/service/discover.go` | Offers one candidate at a time, drawn at random; waits for a registration once exhausted |
| `Watch`    | `internal/service/watch.go`    | Holds the stream; tells a 1/N share of holders when a new address registers |

There is no `Deregister`. Closing the registration stream is what removes the
rows, so a caller that crashes and one that exits cleanly take the same path.

The server also implements standard diagnostic services defined in
`grpc-protos`:

- **Info Service**: `internal/service/get_info.go`
- **Diagnostics Service**: `internal/service/get_diagnostics.go`

### Security Implementation

This implementation uses gRPC peer context to extract the caller's IP during
`Register`. The peer context contains TCP connection metadata that cannot be
spoofed, preventing address hijacking.

The port comes from the caller, read from its own listener, because the peer
socket carries only the ephemeral port it dialed from. See
`internal/service/register.go` for how the two halves are composed.

## Storage Backends

grpcd uses an [abstract storage interface](internal/storage/store.go) that
supports multiple backends.

Nothing takes a TTL. `AddressesFor` returns a sequence rather than a slice, and
each pull is one uniform random draw over the set as it is at that moment, so a
lookup costs O(1) per candidate however many addresses the method has and a
removed address is never drawn again. The sequence ends when the set is empty.

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
- Publish/subscribe, which is how an instance is told a row it anchors was
  removed, and how one subscription per instance wakes every `Discover`
  waiting on a registration
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

### Code Quality

The codebase includes:

- Unit tests for all RPC methods
- Fuzz tests for request validation
- Structured logging with contextual information
- OpenTelemetry metrics for monitoring
- Input validation and error handling

## Related Projects

- [grpcd-protos](https://github.com/sonic-original-software/grpcd-protos):
  Protocol buffer definitions and service specification
- [grpcd-go](https://git.sonicoriginal.software/grpcd-go): Go client library for
  gRPC Method Discovery

## Contributing

Contributions welcome! This is the reference implementation, so changes should
align with the grpcd specification in grpcd-protos.
