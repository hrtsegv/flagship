# Flagship project context

Implementation notes for future development sessions, originally explored on 2026-10-11
at revision `df6d52c` (`Lock the flag row in Update and Delete's read-modify-write`).
Behavior notes are updated as changes land; verify the current implementation
before changing it. Start with [AGENTS.md](../AGENTS.md) for working guidance.

The [README](../README.md) owns the product description and roadmap. The
[contributing guide](../CONTRIBUTING.md) owns local setup and coding conventions.
This file supplies navigation and details that are easy to miss in those guides.
The [completion audit](COMPLETION_AUDIT.md) records remaining gaps and release gates.

## Product direction

The project goal, updated on 2026-10-14, is a complete and reliable platform
deployed as one active Flagship server backed by PostgreSQL. Application
instances continue to evaluate flags locally with the Go SDK. Live subscriber
delivery stays in process; horizontal scaling, cross-instance fan-out, and
distributed coordination are outside scope. The README's
[goal and completion criteria](../README.md#project-goal-and-scope) and
[roadmap](../README.md#roadmap) define the remaining work. The implementation
notes below remain a baseline, not a claim that those capabilities exist.

## Shape of the project

- A single Go module, `github.com/hrtsegv/flagship`, with `go 1.26.6` at the
  explored revision. Direct dependencies are `pgx/v5` and `golang-migrate/v4`;
  SDK production files use the standard library.
- The root package is the public SDK. The control plane lives in `internal/`
  and is assembled by [cmd/flagship/main.go](../cmd/flagship/main.go).
- There is no frontend, generated API client, Makefile, or container/Compose
  configuration at this revision. The runnable demo is [example/main.go](../example/main.go).
- The implemented feature set is projects, environments, boolean flags,
  durable configuration snapshots/events, and resumable SSE synchronization.
  The README tracks planned capabilities.

## Follow the main flows

**Flag update:** [httpapi/server.go](../internal/httpapi/server.go) routes
to [flag_handlers.go](../internal/httpapi/flag_handlers.go), which decodes a
request and calls [app/flag_service.go](../internal/app/flag_service.go).
The service validates/trims input and delegates to the interface in
[app/ports.go](../internal/app/ports.go). The repository applies
[domain.FlagPatch](../internal/domain/flag.go), persists the mutation, advances
the configuration version, records an event, then publishes it to subscribers.

**Snapshot fetch:** the configuration handler calls `FlagService.Snapshot`,
which checks the environment exists and requests a consistent version + flag
set from the repository. [httpapi/dto.go](../internal/httpapi/dto.go) reduces
this to SDK-facing fields. [fetch.go](../fetch.go) decodes that wire shape into
an immutable [Snapshot](../snapshot.go).

**Live SDK synchronization:** [sync.go](../sync.go) bootstraps an unconfigured
client using `FetchSnapshot`, connects to the environment's SSE endpoint,
and calls [Client.ApplyEvent](../client.go) for each recognized SSE frame.
A forward version gap triggers a full fetch on the same connection; subsequent
stale events are ignored until sequential application resumes. Dropped
connections retry with full-jitter exponential backoff, defaulting to a 500 ms
initial ceiling and 30 s maximum. `Sync` blocks in its caller's goroutine and
returns on context cancellation or `ErrWrongEnvironment`; `OnError` reports
transient failures before retry.

An unreplayable subscription sends `configuration.reset` with the environment
key. `Sync` immediately fetches a snapshot and continues on the same stream.
A full live buffer closes and removes that subscription, making the SDK
reconnect and recover even when the dropped event was the last mutation.

**Evaluation:** `Client.Bool` loads an `atomic.Pointer[Snapshot]`, looks up a
boolean in a private map, and returns the caller's fallback if needed. An
explicit `false` value is a configured value. `UpdateSnapshot` and `ApplyEvent`
use compare-and-swap loops; event application copies the map rather than
mutating data readers may still hold.

## Data model and persistence

The hierarchy is `Project -> Environment -> Flag`. IDs are server-generated
UUIDv4-formatted strings using `crypto/rand` in
[internal/idgen](../internal/idgen/idgen.go). Services trim keys, names, and
descriptions; key/name validation currently requires only nonblank text.

| State | Scope and version behavior |
| --- | --- |
| Project key | Unique globally |
| Environment key | Unique within a project |
| Flag key | Unique within an environment; reusable after deletion |
| Flag version | Starts at 1; increments on every update, including empty patches |
| Configuration version | Starts at 0 for an environment; increments on every flag create/update/delete |
| Config event | Identified by environment ID + configuration version; created/updated/deleted event types |

[0001_initial_schema.up.sql](../internal/postgres/migrations/0001_initial_schema.up.sql)
creates `projects`, `environments`, `flags`, and `config_events`, with foreign
keys and scoped uniqueness constraints. Configuration versions live on the
environment row, not in the `domain.Environment` struct. Migrations are embedded
and applied by [postgres.Migrate](../internal/postgres/migrate.go) at server
startup, before opening the long-lived pool.

In [postgres/flag_repository.go](../internal/postgres/flag_repository.go):

- A mutation, environment version increment, event insert, and history trim
  share one transaction. Publication occurs after commit.
- Updates and deletes lock the flag row with `FOR UPDATE` so read-modify-write
  operations cannot lose increments or record duplicate deletions across
  separate connections/processes.
- Snapshots read version and flags in a repeatable-read transaction.
- A repository mutex serializes transaction + local publication against
  replay read + subscriber registration. This protects the replay/live handoff
  within that repository instance.

[memory/flag_repository.go](../internal/memory/flag_repository.go) preserves
the same version/event/subscription contracts using an `RWMutex`, maps, and
channels. Its snapshot captures version and flags under one read lock. When
changing repository contracts, update both implementations and their tests.

## HTTP and wire contract

Routes are registered explicitly with method-aware `http.ServeMux` patterns in
[server.go](../internal/httpapi/server.go). All paths below start with `/api/v1`.

The operational routes `GET /healthz` and `GET /readyz` are outside that prefix.
Liveness does not query PostgreSQL. Readiness invokes the injected database
check with a two-second deadline and returns 503 if it fails or is unconfigured.

| Methods | Path | Purpose |
| --- | --- | --- |
| POST, GET | `/projects` | Create/list projects |
| POST, GET | `/projects/{projectID}/environments` | Create/list environments |
| POST, GET | `/environments/{environmentID}/flags` | Create/list flags |
| GET | `/flags/{flagID}` | Get a flag |
| PATCH | `/flags/{flagID}` | Update name, description, and/or enabled |
| DELETE | `/flags/{flagID}` | Delete a flag; success returns 204 |
| GET | `/environments/{environmentID}/configuration` | Authoritative full configuration |
| GET | `/environments/{environmentID}/stream` | Replay and live SSE |

Create responses use 201; other JSON successes use 200. Lists are JSON arrays.
Flag patches use pointer fields so an omitted field differs from an explicit
`false` or empty string. Keys cannot be changed by the patch API.

[respond.go](../internal/httpapi/respond.go) maps wrapped domain errors using
`errors.Is`: not found -> 404, duplicate key -> 409, validation -> 400, other
errors -> 500. Handler-generated JSON errors use
`{"error":{"code":"...","message":"..."}}`; malformed JSON also returns 400.
Mutation bodies must be one JSON object with known fields. The shared decoder
in [request.go](../internal/httpapi/request.go) rejects `null`, trailing data,
and bodies over 1 MiB (413) before invoking a use case. Empty flag patches are
still valid mutations.

The configuration response has this shape:

```json
{"version":2,"environment":"production","flags":[{"key":"new-checkout","enabled":true}]}
```

An SSE mutation has this shape:

```text
id: 2
event: configuration.updated
data: {"environment":"production","version":2,"type":"flag_updated","flag_key":"new-checkout","enabled":true}

```

Important protocol details:

- URL parameters and `SyncConfig.EnvironmentID` are IDs. The wire field
  `environment`, `Snapshot.Environment()`, and SDK `ConfigEvent.Environment`
  are environment **keys**. Server-side `domain.ConfigEvent` carries the ID;
  the HTTP layer converts it to a key.
- SDK wire structs in `fetch.go`/`sync.go` intentionally mirror, rather than
  import, server DTOs. Keep both sides compatible when changing the protocol.
- The event version is the SSE ID. The SDK resumes with its installed
  snapshot version as `Last-Event-ID`; the parser does not track raw `id:` fields.
- The server validates that `Last-Event-ID` is a nonnegative integer. An
  omitted header means version 0, so retained history can be replayed from 0.
- Both repositories retain at most 1,000 events per environment and buffer 16
  live events per subscriber. A full buffer closes and removes the subscription
  instead of blocking writers. Buffered events drain before EOF; the SDK then
  reconnects using its installed version and replays or fetches a snapshot.
- `SubscribeFrom` returns replay events, a replayable flag, a live channel,
  and an idempotent unsubscribe function that closes and removes the channel.
  The handler always defers unsubscribe. If history is unavailable or the
  requested version is ahead, the HTTP layer sends this frame before live
  events, without an SSE ID or a configuration-version advance:

  ```text
  event: configuration.reset
  data: {"environment":"production"}

  ```

  The SDK fetches the authoritative configuration immediately, checks its
  environment against the frame, and continues on the same connection.
- The server sends comment heartbeats every 20 seconds. The SDK ignores
  comments and SSE event names other than `configuration.updated`.

## Test navigation

| Concern | Existing tests |
| --- | --- |
| Fallbacks, atomic replacement, concurrent evaluation | Root `client_test.go`, `snapshot_test.go` |
| Sequential/stale/gap/wrong-environment/delete events | Root `event_test.go` |
| Fetch errors and recovery against the actual HTTP API | Root `fetch_test.go` |
| SSE parser and jitter/backoff | Root `sse_test.go`, `backoff_test.go` |
| Bootstrap, cancellation, live updates, reconnects, same-connection recovery | Root `sync_test.go` |
| Entity rules and service validation/parent checks | `internal/domain/*_test.go`, `internal/app/*_test.go` |
| Routes, JSON errors, CRUD, snapshots, replay/live streaming | `internal/httpapi/server_test.go` |
| Probe isolation, readiness failures/deadlines | `internal/httpapi/health_test.go` |
| Runtime config, SSE shutdown, bounded forced shutdown | `cmd/flagship/main_test.go` |
| Repository uniqueness, versions, concurrency, retention, replay/live handoff | `internal/memory/*_test.go`, `internal/postgres/*_test.go` |

SDK/API integration tests use `httptest` and the real application/HTTP layers
backed by memory repositories; they do not require PostgreSQL. Repository
handoff tests include concurrency coverage to catch gaps and duplicates.

Useful focused runs from the root:

```bash
go test . -run 'TestClient|TestSnapshot|TestNewSnapshot'
go test . -run 'TestSync|TestFetchSnapshot|TestApplyEvent'
go test ./internal/app/... ./internal/httpapi/...
go test ./internal/memory/...
go test ./internal/postgres/...  # requires FLAGSHIP_TEST_DATABASE_URL to exercise DB tests
```

[postgres/testutil_test.go](../internal/postgres/testutil_test.go) skips tests
when `FLAGSHIP_TEST_DATABASE_URL` is absent, migrates once per test binary,
and truncates all four application tables before each test. Use a disposable
database and do not run concurrent suites against the same database.

[CI](../.github/workflows/ci.yml) uses the Go version from `go.mod`, a PostgreSQL
16 service, and `golangci-lint` without a repository-specific configuration.
Checks are formatting, vet, lint, build, tests, and race tests.

## Implementation limits to remember

- PostgreSQL state is durable, but live delivery is in-process. Separate
  server/repository instances do not receive each other's publications. One
  active server is the supported deployment; horizontal scaling is outside scope.
- The server requires `DATABASE_URL` and runs migrations automatically.
  `HTTP_ADDR` defaults to `:8080`; `SHUTDOWN_TIMEOUT` defaults to `10s`.
  SIGINT/SIGTERM cancels request contexts, including SSE, and initiates bounded
  shutdown before the database pool closes. Header/read/write/idle timeouts are
  5/15/30/60 seconds. SSE renews its 30-second write deadline per frame.
- Authentication, targeting/rollouts, and operational features remain roadmap
  work; consult the README for the current list before assuming they exist.
- `UpdateSnapshot` rejects a different environment once the client is configured.
  `ApplyEvent` checks the environment before the version, and rejects unsupported
  next-version mutation types or blank flag keys with `ErrInvalidEvent`.
  `Sync` recovers invalid mutations through a full snapshot. `FetchSnapshot`
  requires a nonnegative version, environment, flags array, and unique flag keys
  with explicit boolean values; trailing JSON values are rejected.

## Exploration baseline

The installed toolchain was Go 1.26.6 with cgo enabled. `go test ./...` and
`go vet ./...` passed; `gofmt -l .` printed no files.
`FLAGSHIP_TEST_DATABASE_URL` was unset, so PostgreSQL integration tests skipped;
their behavior was inspected in code rather than exercised against a database.
Race tests and `golangci-lint` were not run during this documentation task.
Recheck validation results when making subsequent changes.
