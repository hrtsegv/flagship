# Single-instance completion audit

Reviewed on 2026-10-14 against the code and tests. This is an implementation
assessment, not a claim that Flagship is ready for release. The
[README](../README.md#roadmap) owns milestone order and project scope.
The supported deployment is one active Go server with PostgreSQL.

## Foundation and repairs verified

| Area | Evidence |
| --- | --- |
| Flag management | Project/environment creation and listing; flag CRUD, key reuse, validation, versions, and HTTP errors have tests. |
| Durable configuration | Mutations, environment version increments, and replay events share a PostgreSQL transaction. Snapshot reads are consistent. |
| Local evaluation | Boolean reads use immutable snapshots behind an atomic pointer and preserve caller fallbacks. Normal evaluation does no network I/O. |
| Replay and recovery | Replay/live handoff is atomic. Expired history emits a reset; overflow disconnects only the slow subscriber. Regression tests prove recovery without another mutation. |
| SDK state integrity | Cross-environment replacement is rejected. Invalid next-version mutation types/keys cannot advance state. Incomplete or malformed snapshots are rejected. |
| HTTP input handling | All mutation handlers reject unknown fields, extra JSON, null bodies, and bodies over 1 MiB before changing state. Empty flag patches remain valid. |
| Runtime operation | Configurable listening, bounded HTTP I/O, renewable SSE write deadlines, signal-driven shutdown, lifecycle logs, and liveness/readiness checks are implemented. |

Validation includes normal and race tests with a disposable PostgreSQL 16
database, vet, formatting, and golangci-lint v2.12.2. Regression tests for
stale-client recovery and SDK validation failed before the corresponding fixes.
A compiled-binary smoke test also verified API writes, persisted configuration
after a full process restart, clean SIGTERM with an active SSE stream, and
readiness failure/recovery during a real database pause.

## Remaining implementation gaps

### SDK transport resilience

- `Sync` uses the caller's HTTP client or `http.DefaultClient`; bootstrap,
  snapshot recovery, and response-header waits have no SDK-owned timeout.
  A silent connection can hold synchronization indefinitely.
- SSE heartbeats are ignored, with no client inactivity watchdog. Frame and
  configuration response sizes are unbounded. Mutation payload decoding does
  not yet distinguish all missing fields from zero values.
- Validate nil clients and invalid URLs/configuration before starting work.
  Define behavior for persistent authorization/not-found responses when
  credentials are introduced.

Acceptance: blackholed bootstrap, recovery, and streams retry within documented
deadlines while local reads retain the last valid state; cancellation leaves no
background readers; malformed/oversized input never advances a version.

### Authentication and authorization

There is no credential model, authenticated request identity, route access
control, credential rotation/revocation, or built-in SDK credential option.
Implement authenticated administration and environment-scoped read access to
configuration/streams. Keep credential storage and verification separate from
flag evaluation.

Acceptance: anonymous and wrongly scoped requests fail; SDK readers cannot
mutate flags or enumerate other environments; revoked credentials stop working;
authenticated snapshot fetch and reconnect work end to end. Dashboard login
must use the same authorization rules.

### Targeting and percentage rollouts

Domain flags, SQL rows, DTOs, SDK flags, snapshots, and mutation events currently
carry only a boolean evaluation value. There is no attribute context, rule
representation, evaluator, or rollout policy.

Define rule ordering, supported attribute types/operators, missing-attribute
behavior, and the relationship between targeting and the flag's default or
disabled value. Extend persistence and both sync payloads together. Copy nested
rule data when constructing snapshots. Percentage rollouts need a documented
stable hash/bucket contract and a caller-supplied subject identifier.

Acceptance: snapshot and event evaluation agree; all evaluation stays local;
attribute/fallback edge cases, 0%/100% boundaries, stable assignments across
processes, and increasing rollout percentages have deterministic tests.

### Durable audit history and rollback

`config_events` is a bounded replay log of 1,000 events per environment. It has
no actor identity or full before/after state and cannot reconstruct metadata,
deleted flags, or arbitrary historical configurations.

Add durable history separate from replay retention, recorded atomically with
each mutation. Capture the state needed to inspect and undo a change, including
future rules/rollouts. Rollback must create a new mutation and version.

Acceptance: history survives replay trimming and restart; create/update/delete
and rollback are attributed and inspectable; failures cannot leave orphan
history or partial state; rollback reaches already connected SDKs.

### Dashboard and management lifecycle

There is no frontend or static asset serving. Build the dashboard into the
single-server deployment and use the authenticated API for project/environment
navigation, flag changes, targeting, rollouts, and history/rollback.
Projects and environments currently support creation/listing only; define and
implement their metadata-editing and archival/deletion policy without losing
referential integrity or audit history.

Acceptance: a user can complete these workflows in a browser, including useful
validation/error states and credential expiry, with corresponding API tests.

### Operational visibility and release readiness

- Lifecycle logs and probes exist; request/error logs, metrics, and sync/replay/
  overflow diagnostics still need implementation. Avoid resource IDs and
  credentials in metric labels; preserve SSE flushing through middleware.
- Replay database errors currently become `replayable=false`, hiding the cause.
  Surface repository failures separately from unavailable history.
- Bound startup/database/migration waits and document failed/dirty migration
  recovery. Define supported deployment configuration and a reproducible install.
- Add upgrade, backup, restore, and incident instructions. Restores must preserve
  version monotonicity or explicitly rebootstrap SDK clients.
- Measure flag count, mutation rate, concurrent streams, memory use, and recovery
  under load. Current list endpoints are unpaginated and mutations serialize
  behind one repository lock; document capacity and address measured limits.

Acceptance: operators can diagnose a failing request or stale SDK, recover a
backup into a fresh database, upgrade an existing installation, and run within
published capacity limits. Release checks must exercise the authenticated
browser/API/SDK workflow with PostgreSQL and the race detector.

Horizontal scaling, shared event brokers, clustered servers, leader election,
and cross-region replication are outside this audit's completion gates.
