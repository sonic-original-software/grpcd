# grpcd server

The grpcd server. It answers the three RPCs in
[grpcd/protos](https://github.com/grpcd/protos), holds the streams that are
registrations, and keeps the rows in a storage backend shared by every instance
in a region.

Container images are published to `ghcr.io/grpcd/server`.

## Quick Start

```bash
# In-memory storage (development)
docker run -p 5000:5000 ghcr.io/grpcd/server:latest

# Redis (production)
docker run -p 5000:5000 \
  -e GRPC_MAX_CONNECTION_AGE=0 \
  -e STORAGE_BACKEND=redis \
  -e STORAGE_ADDRESS=redis:6379 \
  ghcr.io/grpcd/server:latest
```

## Configuration

Everything is an environment variable.

| Variable                  | Description                                              | Default |
| ------------------------- | -------------------------------------------------------- | ------- |
| `GRPC_SERVER_ADDRESS`     | Address to bind                                          | `:5000` |
| `GRPC_MAX_CONNECTION_AGE` | Age at which the server sends a GOAWAY. `0` is no limit. | `10m`   |
| `STORAGE_BACKEND`         | `redis`, or empty for in-memory                          | -       |
| `STORAGE_ADDRESS`         | Storage backend address. Required for `redis`.           | -       |

Set `GRPC_MAX_CONNECTION_AGE=0`. A registration lives as long as the stream
holding it, and a GOAWAY ends that stream whether or not it is active. At the
ten minute default, every registration in the mesh is torn down and rebuilt on
that timer. It works, and it looks healthy, while costing a reconnect per
service per interval. The server reports the value in effect at startup.

### Health

The health service answers for two entries. `""` says the process is alive.
`grpcd.GRPCDService` says whether the store can be reached: it reports
`NOT_SERVING` from the first operation that fails against the store until the
store's subscription comes back, and `SERVING` otherwise.

A balancer or orchestrator probe that should route around an instance that has
lost its store asks for `grpcd.GRPCDService`. A probe that asks for `""` sees
only whether the process is up.

## Design

### State lives in the storage backend

Every fact the server serves is in the storage backend, so any instance answers
any lookup and a replacement instance serves immediately. An instance
additionally holds the registration streams it accepted, which is what ties a
row's lifetime to its service's.

```
┌────────────────────────────────────────┐
│              grpcd server              │
│                                        │
│   ┌──────────────────────────────┐     │
│   │     Service Layer            │     │
│   │  • Register (held stream)    │     │
│   │  • Discover (bidirectional)  │     │
│   │  • Watch (held stream)       │     │
│   │  • Validation                │     │
│   └──────────┬───────────────────┘     │
│              │                         │
│   ┌──────────▼───────────────────┐     │
│   │   Storage Interface          │     │
│   └──────────┬───────────────────┘     │
│              │                         │
└──────────────┼─────────────────────────┘
               │
     ┌─────────┴─────────┐
     │                   │
 ┌───▼────┐      ┌───────▼────┐
 │ Redis  │      │    Mock    │
 │Backend │      │  (Testing) │
 └────────┘      └────────────┘
```

The backend owns persistence and replication. Embedded storage would need Raft
for HA; in-memory with peer sync would need gossip and reconciliation; a direct
Redis dependency would be untestable without Redis. The interface keeps the
server to the business logic and leaves storage HA to whoever runs the store.

### Data model

Two mappings, neither with an expiry:

- **method → addresses.** Key: the method name. Value: the set of addresses.
  Read by `Discover`; its cardinality is the `N` a `Watch` draws against.
- **address → anchor.** Key: the address. Value: the id of the instance holding
  that address's `Register` stream. Read to address the notification when a row
  is removed.

An instance generates a UUIDv4 at startup and uses it as its anchor id and as
its notification channel. The id names a channel that lives and dies with the
process, so it needs no coordination and no durability.

### Registration

`internal/service/register.go`. The handler validates the request, reads the
IP off the peer, composes the address, writes one row per method plus the
anchor, sends the acknowledgement, and blocks on the stream. When the stream
ends it removes every row the request named. The handler holds that list for
the life of the stream, so no reverse mapping is stored.

### Discovery

`internal/service/discover.go`. `AddressesFor` on the store is a sequence, and
each pull is one uniform random draw over the set as it is at that moment, so
a lookup costs O(1) per candidate however many addresses the method has and a
removed address is never drawn again. The sequence ends when the set is empty,
and the handler then waits for the next addition announced for the method.

### Rebalancing

`internal/service/watch.go`. The handler sleeps until an addition is announced.
On one for its method whose address differs from the one held, it draws with
probability `1/N` and sends the new address only if it wins. Two additions back
to back can cost one missed rebalance, which the next addition corrects; the
store's set is the truth throughout.

### Notifications

Every registration is announced once per instance, over one subscription to
the storage backend, and every handler waiting on that instance is woken by
that one announcement. Each woken handler reads the store to learn whether the
registration concerned it. No instance holds a list of who is waiting for what.

Instances also notify each other about removals, over the backend's
publish/subscribe, addressed to a single anchor.

### Reverting a wrong removal

The instance anchoring an address is notified when that row is removed and
writes it back. Its open `Register` stream is live proof the service is up, so
it performs no check of its own. `grpcd.removals.reverted.total` counts these,
so a client with a persistent local fault surfaces in monitoring.

### Losing the storage backend

`internal/service/outage.go`. An instance that cannot reach the backend can
neither record a registration nor answer a lookup, and is deaf to additions. It
keeps serving and reports `grpcd.GRPCDService` as `NOT_SERVING` until the
backend's subscription comes back.

The streams it holds are kept. A handler that fails against the backend waits
for it to return and carries on: a registration is written once it can be, a
lookup draws again, a watch resumes. A client already on the instance sees a
call take longer and nothing else.

When the backend returns, every held registration writes its rows again from
the request the handler still holds, so a backend that came back empty is
repopulated by the instances themselves. Waiting lookups draw again, since the
backend may hold registrations the instance was deaf to.

## Failure Modes

**Service crash.** Its `Register` stream ends and its address is removed at
once. Clients holding a connection to it see that connection break and
rediscover.

**Instance crash.** Its rows remain, because removal happens when an instance
observes a stream ending and this one is gone; live services stay discoverable
throughout. Its registration streams break, and each service reconnects to
another instance and re-registers: the same rows, written again, with the
anchor overwritten by the new instance's id. Until that lands, those rows carry
the id of a channel nobody reads, so a wrong `dead_address` report in that
window drops a live service until it re-registers.

**Service and its anchoring instance crash together.** Nothing runs the
removal, so the row remains. The first client to discover that address fails
against it and reports it, which removes it.

**Storage backend failure.** Covered under Design. Services and clients continue
on the connections they already hold.

## Storage Backends

`internal/storage/store.go` is the interface. Adding a backend means
implementing it and adding a case to `internal/storage/resolver/resolve.go`.

**In-memory** (`internal/storage/mock/`). Selected when `STORAGE_BACKEND` is
unset. No external dependency; state is lost on restart, and instances do not
share it.

**Redis** (`internal/storage/redis/`). The production backend. Sets hold the
method rows, and publish/subscribe carries the addition announcements and the
removal notifications. Instances share state, and Sentinel or Cluster supply
HA.

## Observability

Counters, exported over OpenTelemetry:

- `grpcd.registrations.total`
- `grpcd.removals.total`
- `grpcd.discoveries.total`
- `grpcd.removals.reverted.total`
- `grpcd.rebalances.total`

The info and diagnostics services from
[grpc-service](https://github.com/sonic-original-software/grpc-service) report
the version and the store's connectivity.

## Development

```bash
go test ./...
go test -fuzz=FuzzRegister ./internal/service
go test -fuzz=FuzzDiscover ./internal/service
CGO_ENABLED=0 go build -ldflags="-w -s" -o grpcd .
```
