<p align="center">
  <img src="assets/logo.svg" alt="Flagship logo: a ship carrying a Go-gopher-style mascot, flying a toggle-switch flag from its mast" width="220">
</p>

<h1 align="center">Flagship</h1>

<p align="center">
  <a href="https://github.com/hrtsegv/flagship/actions/workflows/ci.yml"><img src="https://github.com/hrtsegv/flagship/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
</p>

Flagship is a self-hosted feature-flag platform written in Go. It lets a team
change application behavior at runtime — enabling a feature, rolling it back,
ramping it out gradually — without redeploying.

> **Status:** actively in development. The control plane (projects,
> environments, boolean flags) is backed by PostgreSQL, a local-evaluation Go
> SDK and a versioned config-sync protocol with a live SSE push transport are
> working end-to-end — server and SDK client both. See
> [Roadmap](#roadmap) for what's next.

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

**Control plane** — an HTTP API backed by a PostgreSQL repository (an
in-memory implementation of the same interfaces also exists and is what the
fast unit test suite uses, but the server binary requires PostgreSQL):

- Create/list **projects**
- Create/list **environments** (scoped to a project)
- Create/list/get/update/delete **boolean feature flags** (scoped to an
  environment) — a deleted flag's key becomes reusable
- Server-generated IDs, enforced key-uniqueness invariants, and a
  version counter that increments on every flag mutation, including deletes
- `GET .../configuration` — an environment's full current config plus a
  configuration version (its own counter, independent of any single flag's
  version) — the sync protocol's authoritative recovery endpoint, below
- `GET .../stream` — a live Server-Sent Events feed of that environment's
  configuration changes, with resume-after-disconnect via `Last-Event-ID`
  and no gap or duplicate at the replay→live handoff
- Consistent JSON error responses (`404` for missing resources, `409` for
  key conflicts, `400` for validation failures)

**Go SDK** (root package, `flagship`) — local, concurrency-safe boolean flag
evaluation, plus the client-side half of the versioned sync protocol:

- `client.Bool(key, default)` never makes a network call and never returns a
  zero value in place of your fallback
- An immutable, versioned configuration `Snapshot` that the client holds
  behind an atomic pointer — safe to read from many goroutines while a new
  snapshot is being installed
- `Client.ApplyEvent` applies one versioned `ConfigEvent` at a time, strictly
  sequentially: stale/duplicate events are ignored, and any gap (or no
  snapshot yet) reports `ErrVersionGap` instead of guessing
- `FetchSnapshot` — the one recovery mechanism for every kind of gap: fetch
  the authoritative configuration and install it wholesale
- `flagship.Sync` — run it in a goroutine and it connects to `/stream`,
  applies events as they arrive, recovers from a gap on the *same*
  connection (no reconnect needed), and reconnects with jittered
  exponential backoff if the connection drops

**What's intentionally not built yet:** horizontal scaling across multiple
server instances, observability, authentication, targeting rules, percentage
rollouts, audit history, and a dashboard. See [Roadmap](#roadmap).

## Quick start

The server requires PostgreSQL — point `DATABASE_URL` at one (pending schema
migrations run automatically on startup):

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/flagship?sslmode=disable
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

curl -s localhost:8080/api/v1/environments/<environmentID>/configuration
# {"version":2,"environment":"production","flags":[{"key":"new-checkout","enabled":true}]}

curl -s -N localhost:8080/api/v1/environments/<environmentID>/stream
# id: 1
# event: configuration.updated
# data: {"environment":"production","version":1,"type":"flag_created","flag_key":"new-checkout","enabled":false}
#
# (stays open; further flag changes appear here live, in real time)
```

From a Go application, `flagship.Sync` is the client for that stream —
bootstrap, connect, and keep applying events, all in one call:

```go
client := flagship.NewClient(nil)
go flagship.Sync(ctx, client, flagship.SyncConfig{
	BaseURL:       "http://localhost:8080",
	EnvironmentID: environmentID,
})
// client.Bool("new-checkout", false) now reflects live changes to the
// flag, made through the API above, with no restart and no polling.
```

[`example/`](example/) is a small, runnable version of exactly this: it
connects, prints a flag's value, and keeps printing it live as you flip it
through the API — no restart, no polling.

```bash
go run ./example -environment=<environmentID>
# in another terminal: curl -X PATCH .../flags/<flagID> -d '{"enabled":true}'
# the running example prints the new value immediately
```

`FetchSnapshot` alone is still there for a one-shot fetch with no live
updates, and a `Snapshot` can be constructed directly, which is enough to
evaluate flags locally without a server at all:

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
(internal/httpapi) (internal/app)            (internal/domain) (internal/postgres, internal/memory)
```

- **`internal/domain`** — `Project`, `Environment`, `Flag`, and the
  invariants that must hold regardless of transport or storage (unique keys,
  a flag versions up on every update, etc).
- **`internal/postgres`** — the repository implementation the server binary
  actually runs on: schema migrations, a connection pool, and one repository
  per aggregate, each mutation and its config-event/version bump committed
  in a single transaction.
- **`internal/memory`** — a mutex-guarded, in-memory implementation of the
  same repository interfaces, kept around because it's what the fast,
  DB-free unit test suite (and `internal/app`/`internal/httpapi`'s own
  tests) constructs directly.
- **`internal/app`** — orchestrates use cases: validates input, checks that
  a parent resource exists before creating a child, assigns server-side IDs.
  Depends only on the repository interfaces, not on either implementation.
- **`internal/httpapi`** — decodes/encodes JSON and maps domain errors to
  HTTP status codes. No domain logic lives here.

The **SDK** (root package) is deliberately independent of all of the above —
it has no import of `internal/httpapi` or `internal/memory`, and never will.
It's the data-plane half of the system: give it a `Snapshot`, and it
evaluates flags against it in memory. How that snapshot gets there —
constructed directly, fetched once with `FetchSnapshot`, or kept live with
`Sync` — is a separate concern the SDK doesn't need to know about.

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
  older or equal-version snapshot is silently rejected, whether it arrives
  via a full fetch or as one `ConfigEvent` in a sequence.
- **A gap is a gap, however it happened.** `ApplyEvent` doesn't distinguish
  "missed one update" from "brand new client" — both fail the same
  sequential check and get the same fix: fetch a full snapshot and install
  it. One recovery path instead of special-casing every way sync can fall
  behind.
- **Replay and going live are one atomic operation, not two.** A naive
  "read history, then subscribe" has a race: something can be recorded in
  between and land in neither. The server instead registers a live
  subscriber and reads replay history under the same lock recording an
  event uses, so there's no window for a gap or a duplicate at that
  handoff — by construction, not by careful timing.
- **IDs are server-generated**, using a small `crypto/rand`-based generator
  rather than a UUID dependency, since nothing beyond uniqueness and an
  opaque string was needed.

## Running tests

```bash
go test ./...
go test -race ./...   # requires cgo
go vet ./...
```

The PostgreSQL repository tests skip themselves unless `FLAGSHIP_TEST_DATABASE_URL`
points at a real database:

```bash
export FLAGSHIP_TEST_DATABASE_URL=postgres://user:pass@localhost:5432/flagship_test?sslmode=disable
go test ./internal/postgres/...
```

CI runs `go vet`, `gofmt`, `golangci-lint`, and the full test suite
(including the PostgreSQL tests, against a real Postgres service container)
on every push and pull request to `main`.

Tests cover: domain invariants (duplicate keys, version increments),
concurrency safety of the in-memory repositories, the full HTTP
create/get/list/update/delete flow and its error responses, the SDK's
evaluation/fallback/versioning/concurrency behavior (including snapshot
replacement racing against concurrent reads), the sync protocol
(sequential apply, stale/duplicate/gap handling, and gap recovery against
a real running server, not just a mock), the SSE transport (replay, live
delivery, the replay→live handoff proven gap/duplicate-free with a
deterministic concurrency test, slow-consumer non-blocking behavior, and
an end-to-end run against a real streamed HTTP connection), and `Sync`
itself (reconnect-after-drop with backoff, in-place gap recovery proven to
need exactly one connection, failing fast instead of retrying on a
wrong-environment event, and applying a real flag update end-to-end
against the real control plane with no polling).

## Roadmap

Roughly in order:

- [x] Versioned sync protocol foundation (`ApplyEvent`, gap detection, snapshot recovery)
- [x] Server-side SSE transport (`/stream`, event log, replay-on-reconnect)
- [x] SDK-side SSE client (`flagship.Sync` — connect, apply events, reconnect with backoff)
- [x] PostgreSQL persistence for the control plane
- [x] Example application demonstrating the full loop end-to-end
- [ ] Horizontal scaling across multiple server instances
- [ ] Observability (logging/metrics/tracing, health checks)
- [ ] Authentication/authorization
- [ ] Targeting rules (evaluate against request attributes like country/plan)
- [ ] Deterministic percentage rollouts
- [ ] Audit history and rollback

## Project layout

```text
cmd/flagship/       entrypoint — go run ./cmd/flagship
example/            runnable demo of the full sync loop — go run ./example
internal/domain/    entities + invariants
internal/postgres/  PostgreSQL repositories, schema migrations
internal/memory/    in-memory repositories (used by the fast test suite)
internal/app/       use cases / services
internal/httpapi/   HTTP handlers, routing, JSON (de)serialization
internal/idgen/     ID generation
client.go           SDK: Client, Bool(), ApplyEvent()
snapshot.go         SDK: Snapshot, Flag
event.go            SDK: ConfigEvent, the sync protocol's wire event
fetch.go            SDK: FetchSnapshot — bootstrap + gap recovery
sse.go              SDK: Server-Sent Events frame parsing
backoff.go          SDK: jittered exponential backoff for reconnects
sync.go             SDK: Sync — the SSE client, wired to ApplyEvent
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for a fuller tour of the codebase and
how to get a local dev environment running.
