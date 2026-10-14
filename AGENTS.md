# Working in Flagship

## Start here

Flagship is a self-hosted feature-flag platform with a PostgreSQL-backed Go
control plane and a Go SDK that evaluates boolean flags locally. This is one
Go module, `github.com/hrtsegv/flagship`; use the Go version in `go.mod`.

The goal is a complete, reliable single-instance platform: one active Flagship
server backed by PostgreSQL, with local SDK evaluation in application instances.
Keep live delivery in process. Horizontal scaling, cross-instance event fan-out,
and distributed coordination are outside scope. Use the README's completion
criteria and roadmap to guide new work.

Read [docs/PROJECT_CONTEXT.md](docs/PROJECT_CONTEXT.md) for the implementation
map and protocol details. [README.md](README.md) describes the product and
roadmap; [CONTRIBUTING.md](CONTRIBUTING.md) covers development conventions and
setup. Check the current code and Git diff before relying on these notes.

## Where changes belong

| Area | Location |
| --- | --- |
| Public SDK: evaluation, immutable snapshots, event application | Root `client.go`, `snapshot.go`, `event.go` |
| SDK: snapshot fetch, SSE parsing, reconnect and recovery | Root `fetch.go`, `sse.go`, `backoff.go`, `sync.go` |
| Server startup and dependency wiring | `cmd/flagship/main.go` |
| Runnable SDK demonstration | `example/main.go` |
| Entities, patches, validation, sentinel errors | `internal/domain/` |
| Use cases and repository interfaces | `internal/app/`, especially `ports.go` |
| Routes, handlers, JSON DTOs, error mapping, SSE responses | `internal/httpapi/` |
| Production persistence and embedded SQL migrations | `internal/postgres/` |
| Concurrency-safe repositories used by DB-free tests | `internal/memory/` |
| Random resource IDs | `internal/idgen/` |
| CI checks | `.github/workflows/ci.yml` |

The server uses the standard library HTTP router. Application services depend
on repository interfaces; both storage implementations must preserve their
contracts. Keep HTTP concerns in `httpapi`, use-case orchestration in `app`,
and entity rules in `domain`. SDK production code stays independent of
`internal/*`; SDK integration tests may construct the real server internally.

## Invariants to preserve

- `Client.Bool` performs only local reads and honors the caller's fallback
  when configuration or the flag is missing. Network synchronization is
  separate from evaluation.
- Snapshots are immutable, copied on construction/application, and installed
  atomically. A replacement must have a strictly newer configuration version.
- Within an environment, a configuration version identifies one complete
  flag state. Per-flag versions and environment configuration versions are
  separate counters.
- Every successful flag create, update, or delete increments the environment
  configuration version and records its event atomically. Even an empty or
  metadata-only patch increments versions and emits an update event.
- `ApplyEvent` applies only the next version. Stale/duplicate events for the
  same environment are no-ops; a missing snapshot or a forward gap requires a
  full snapshot fetch. Preserve wrong-environment error handling.
- Replay history and live subscription registration must stay atomic with
  respect to mutations. Slow subscribers must not block writers. Preserve
  unsubscribe cleanup and snapshot-based recovery.
- Project keys are globally unique; environment keys are unique per project;
  flag keys are unique per environment. Deletion frees a flag's key for reuse.
- API paths and SDK `EnvironmentID` use the opaque environment ID. Snapshot
  and SSE payloads identify the environment by its **key**.

## Run and validate

Run commands from the repository root:

```bash
go run ./cmd/flagship            # requires DATABASE_URL; listens on :8080
go run ./example -environment=<environmentID>
go test ./...
go test -race ./...             # requires cgo
go vet ./...
gofmt -l .                     # expected to print nothing
golangci-lint run ./...         # when installed; CI also runs it
```

The server applies embedded migrations on startup and requires PostgreSQL;
there is no in-memory server mode. See the existing setup instructions for
starting a local database.

PostgreSQL tests skip unless `FLAGSHIP_TEST_DATABASE_URL` is set. Use a
dedicated disposable test database: `internal/postgres/testutil_test.go`
truncates `projects`, `environments`, `flags`, and `config_events` before each
test. Do not parallelize tests sharing that database. CI uses PostgreSQL 16
and runs build, normal tests, and race tests, plus formatting/vet/lint checks.

Use focused tests for the affected packages, then the broader checks warranted
by the change. Add meaningful tests for new behavior, especially evaluation,
versioning, persistence, and concurrency. Documentation-only changes need
link/content review rather than new Go tests.

## Keep context useful

Commit completed changes in focused commits after validation. Do not add
co-author trailers to commits.

Follow the coding conventions in `CONTRIBUTING.md`: small interfaces, explicit
dependencies, context-aware operations, idiomatic Go, and no speculative
frameworks or abstractions. Update relevant documentation when changing public
behavior, architecture, commands, or protocol semantics. Keep detailed project
knowledge in `docs/PROJECT_CONTEXT.md` and this file concise. Treat dated
observations as a baseline to recheck, not permanent guarantees; keep the
roadmap in the README rather than duplicating it here.
