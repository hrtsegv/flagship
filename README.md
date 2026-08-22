# Flagship

Flagship is a self-hosted feature-flag platform written in Go. It lets a team
change application behavior at runtime — enabling a feature, rolling it back,
ramping it out gradually — without redeploying.

> **Status:** actively in development. The control plane (projects,
> environments, boolean flags) and a local-evaluation Go SDK are working
> end-to-end. See [Roadmap](#roadmap) for what's next.

## Why

Feature flags are often evaluated in hot request paths, so a flag check
should never cost a network round-trip or become a new point of failure.
Flagship's core design constraint is:

> Flag evaluation happens **locally**, inside the application process — never
> as a request to the Flagship server.

The control plane's job is to manage configuration. The SDK's job is to hold
a local copy of that configuration and evaluate it in memory, so the
application keeps working — serving its last-known-good configuration —
even if Flagship itself is temporarily unreachable.

```text
Control Plane                          Application
┌─────────────────────┐                ┌───────────────────────────┐
│ Flagship HTTP API    │  configuration │ Flagship Go SDK           │
│ Projects/Envs/Flags  │ ─────────────► │ Snapshot (version+flags)  │
│ In-memory repository │                │ client.Bool(key, default) │
└─────────────────────┘                └───────────────────────────┘
```

## What's implemented

**Control plane** — an HTTP API backed by a concurrency-safe in-memory
repository:

- Create/list **projects**
- Create/list **environments** (scoped to a project)
- Create/list/get/update **boolean feature flags** (scoped to an environment)
- Server-generated IDs, enforced key-uniqueness invariants, and a
  version counter that increments on every flag update
- Consistent JSON error responses (`404` for missing resources, `409` for
  key conflicts, `400` for validation failures)

**Go SDK** (root package, `flagship`) — local, concurrency-safe boolean flag
evaluation:

- `client.Bool(key, default)` never makes a network call and never returns a
  zero value in place of your fallback
- An immutable, versioned configuration `Snapshot` that the client holds
  behind an atomic pointer — safe to read from many goroutines while a new
  snapshot is being installed
- Version-aware snapshot replacement: a snapshot only replaces the current
  one if its version is strictly newer

**What's intentionally not built yet:** persistence (PostgreSQL), pushing
configuration from the server to the SDK (polling/SSE), targeting rules,
percentage rollouts, authentication, audit history, a dashboard. See
[Roadmap](#roadmap).

## Quick start

Run the server:

```bash
go run ./cmd/flagship
```

Create a project, an environment, and a flag, then read it back:

```bash
curl -s -X POST localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"key":"ecommerce","name":"Ecommerce"}'

curl -s -X POST localhost:8080/api/v1/projects/<projectID>/environments \
  -H "Content-Type: application/json" \
  -d '{"key":"production","name":"Production"}'

curl -s -X POST localhost:8080/api/v1/environments/<environmentID>/flags \
  -H "Content-Type: application/json" \
  -d '{"key":"new-checkout","name":"New Checkout","enabled":false}'

curl -s -X PATCH localhost:8080/api/v1/flags/<flagID> \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}'
```

Use the SDK from a Go application. Today the SDK is fed a snapshot directly
(there's no sync mechanism yet — that's the next milestone), which is enough
to evaluate flags locally:

```go
package main

import (
	"fmt"

	"github.com/hrtsegv/flagship"
)

func main() {
	snapshot := flagship.NewSnapshot("production", 1, []flagship.Flag{
		{Key: "new-checkout", Enabled: true},
	})
	client := flagship.NewClient(snapshot)

	fmt.Println(client.Bool("new-checkout", false)) // true
	fmt.Println(client.Bool("does-not-exist", false)) // false (fallback)
}
```

## Architecture

The control plane follows a straightforward layered structure:

```text
HTTP transport  →  application/use cases  →  domain  →  repository
(internal/httpapi) (internal/app)            (internal/domain) (internal/memory)
```

- **`internal/domain`** — `Project`, `Environment`, `Flag`, and the
  invariants that must hold regardless of transport or storage (unique keys,
  a flag versions up on every update, etc).
- **`internal/memory`** — a mutex-guarded, in-memory repository per
  aggregate. Key-uniqueness checks and inserts happen atomically under a
  single lock.
- **`internal/app`** — orchestrates use cases: validates input, checks that
  a parent resource exists before creating a child, assigns server-side IDs.
- **`internal/httpapi`** — decodes/encodes JSON and maps domain errors to
  HTTP status codes. No domain logic lives here.

The **SDK** (root package) is deliberately independent of all of the above —
it has no import of `internal/httpapi` or `internal/memory`, and never will.
It's the data-plane half of the system: give it a `Snapshot`, and it
evaluates flags against it in memory. How that snapshot gets there (direct
construction today; polling or push-based sync later) is a separate concern
the SDK doesn't need to know about.

## Design decisions worth knowing about

- **Local evaluation is non-negotiable.** `Client.Bool` reads from an
  in-memory snapshot; there is no code path from it to the network. This is
  the whole point of the project, not an optimization.
- **Snapshots are immutable and replaced atomically.** Rather than mutating
  individual flags, the SDK holds a snapshot behind an `atomic.Pointer` and
  swaps it wholesale via compare-and-swap. This means concurrent
  `Bool()` calls never observe a half-updated configuration, and there's no
  lock contention on the read path.
- **Versions are compared, not trusted.** Installing a snapshot only
  succeeds if its version is strictly newer than what's currently held. An
  older or equal-version snapshot is silently rejected — this is what will
  let a future poller apply out-of-order responses safely.
- **IDs are server-generated**, using a small `crypto/rand`-based generator
  rather than a UUID dependency, since nothing beyond uniqueness and an
  opaque string was needed.

## Running tests

```bash
go test ./...
go test -race ./...   # requires cgo
go vet ./...
```

Tests cover: domain invariants (duplicate keys, version increments),
concurrency safety of the in-memory repositories, the full HTTP
create/get/list/update flow and its error responses, and the SDK's
evaluation/fallback/versioning/concurrency behavior (including snapshot
replacement racing against concurrent reads).

## Roadmap

Roughly in order:

- [ ] SDK configuration sync (server → SDK) via polling
- [ ] PostgreSQL persistence for the control plane
- [ ] Push-based updates (SSE) as an alternative to polling
- [ ] Targeting rules (evaluate against request attributes like country/plan)
- [ ] Deterministic percentage rollouts
- [ ] Audit history and rollback
- [ ] Authentication/authorization

This is a learning/portfolio project, not an attempt to match a mature
platform feature-for-feature — see the design principles above for the
constraints guiding what gets built next.

## Project layout

```text
cmd/flagship/       entrypoint — go run ./cmd/flagship
internal/domain/    entities + invariants
internal/memory/    in-memory repositories
internal/app/       use cases / services
internal/httpapi/   HTTP handlers, routing, JSON (de)serialization
internal/idgen/     ID generation
client.go           SDK: Client, Bool()
snapshot.go         SDK: Snapshot, Flag
```
