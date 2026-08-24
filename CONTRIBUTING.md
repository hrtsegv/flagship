# Contributing to Flagship

This is a guide for getting productive in this codebase: how to run it, how
to test it, how the pieces fit together, and what not to break. For the
product pitch — what Flagship is and why it's built the way it is — see
[README.md](README.md) first if you haven't already.

## Prerequisites

- **Go**, matching the version in [go.mod](go.mod).
- **PostgreSQL** — the server binary requires a real database; there's no
  in-memory fallback for `cmd/flagship`. Any Postgres 14+ works. Easiest via
  Docker:

  ```bash
  docker run -d --name flagship-postgres -p 5432:5432 \
    -e POSTGRES_PASSWORD=flagship -e POSTGRES_DB=flagship \
    postgres:16
  ```
- **Git**, obviously, and a GitHub account if you're opening a PR.

## Running it locally

```bash
export DATABASE_URL=postgres://postgres:flagship@localhost:5432/flagship?sslmode=disable
go run ./cmd/flagship
```

Schema migrations run automatically on startup — there's no separate
migration step. The server listens on `:8080`. See the **Quick start**
section of the README for example `curl` commands against it.

## Running the tests

```bash
go test ./...            # fast, no database needed
go test -race ./...      # same, with the race detector (requires cgo)
go vet ./...
gofmt -l .                # should print nothing
```

Most of the suite runs against an in-memory repository and needs nothing
extra. The `internal/postgres` package is the exception: its tests talk to
a real database and skip themselves unless `FLAGSHIP_TEST_DATABASE_URL` is
set:

```bash
export FLAGSHIP_TEST_DATABASE_URL=postgres://postgres:flagship@localhost:5432/flagship_test?sslmode=disable
go test ./internal/postgres/...
```

Point it at a *throwaway* database — these tests truncate every table
between runs.

### Linting

CI runs [golangci-lint](https://golangci-lint.run/). To run it locally:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
golangci-lint run ./...
```

There's no repo-specific `.golangci.yml` — it runs with the tool's default
linter set (errcheck, govet, staticcheck, unused, and friends). If a linter
flags something that's genuinely safe to ignore (a deferred `Close()` after
a transaction is already committed, for example), discard the error
explicitly at the call site — `defer func() { _ = tx.Rollback(ctx) }()` —
rather than adding a blanket exclusion to a config file. It keeps the
reason visible to whoever reads that line next, instead of requiring them
to go check a separate file.

## Continuous integration

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs on every push and
pull request to `main`: a `lint` job (`gofmt`, `go vet`, `golangci-lint`)
and a `test` job that spins up a real PostgreSQL service container and runs
the full suite — including the `internal/postgres` integration tests —
against it. Both must pass before merging.

## Repository tour

The control plane is a straightforward layered structure:

```text
HTTP transport  →  application/use cases  →  domain  →  repository
(internal/httpapi) (internal/app)            (internal/domain) (internal/postgres, internal/memory)
```

Follow a single request through it to see how the layers connect — say,
`PATCH /api/v1/flags/{id}` to flip a flag on:

1. **`internal/httpapi`** decodes the JSON body, calls the application
   layer, and maps whatever error comes back to an HTTP status. It contains
   no domain logic — it doesn't know what makes a flag update valid.
2. **`internal/app`** (`FlagService`) is the use case: it checks the flag
   exists, applies the patch, and delegates persistence to a
   `FlagRepository` — an interface, not a concrete type. This is what lets
   `internal/postgres` and `internal/memory` both satisfy it.
3. **`internal/domain`** defines what a `Flag` is and the invariants that
   must hold regardless of transport or storage — e.g. a flag's own version
   increments on every update.
4. **`internal/postgres`** is the interface's real implementation: it
   updates the flag row, bumps the owning environment's `config_version`,
   and inserts a matching row into `config_events` — all inside one
   transaction, so a reader can never observe a version bump whose event is
   missing, or vice versa. `internal/memory` does the same thing with a
   mutex-guarded map instead of a database, and is what the fast, DB-free
   unit test suite constructs directly.

The **Go SDK** (the root package, imported as `flagship`) is deliberately on
the other side of a hard line: it has no import of `internal/httpapi` or
either repository, and never will. Give it a `Snapshot` and it evaluates
flags against it in memory — how that snapshot arrived (constructed
directly, fetched once, or kept live via `flagship.Sync`) is not its
concern. If you're changing SDK code, you should never need to look at
`internal/*` to understand it, and vice versa.

### Core invariant

**Same environment + same configuration version ⇒ same complete
configuration state.** If two SDK instances report the same version for an
environment, they must have the same effective flag configuration — no
exceptions. This is why every mutation bumps the version and records the
matching event in one transaction, why the SDK only ever accepts a
strictly-newer version, and why any gap (missed event, brand-new client, it
doesn't matter which) is recovered the same way: fetch a full authoritative
snapshot and install it wholesale, never partially reconstruct one. If a
change you're making would require weakening this, stop and think it
through explicitly before writing code — don't quietly relax it to make an
implementation simpler.

### Glossary

- **Project** — top-level grouping; owns one or more environments.
- **Environment** — a deployment target (`production`, `staging`, ...)
  scoped to a project; owns flags and has its own `config_version`.
- **Flag** — a boolean toggle scoped to an environment, with its own
  per-flag version that increments on every update.
- **ConfigEvent** — one recorded mutation (`flag_created`,
  `flag_updated`, `flag_deleted`) tagged with the environment's
  post-mutation `config_version`. The unit the SSE stream and the SDK's
  `ApplyEvent` both operate on.
- **Snapshot** — the SDK's full, immutable, versioned view of one
  environment's flags — what `client.Bool()` actually reads.
- **Gap** — any time an incoming version isn't exactly one more than what
  the SDK currently holds (stale, duplicate, or ahead). Always resolved by
  fetching a fresh snapshot, never by guessing what was missed.

## Coding conventions

- Idiomatic Go: small interfaces, explicit dependencies, context-aware
  operations, table-driven tests where they help.
- No global mutable state, no speculative abstractions, no framework-heavy
  patterns. A one-shot operation doesn't need a helper; three similar lines
  beat a premature abstraction.
- New capabilities need tests alongside them, not after — particularly
  anything touching flag evaluation, configuration versioning, or
  concurrency.
- Don't change established evaluation or sync-protocol semantics silently.
  If a change requires it, that's a design decision worth writing down and
  discussing, not a diff to slip in quietly.

## Commits and pull requests

- Keep commits atomic — one coherent change per commit rather than one
  large commit for unrelated work.
- Write commit messages that explain *why*, not just what changed; the diff
  already shows what changed.
- Make sure `go vet`, `gofmt -l .`, `golangci-lint run ./...`, and
  `go test ./...` are all clean before opening a PR — CI enforces the same
  checks.

## Roadmap

See the [Roadmap](README.md#roadmap) section of the README for what's
already built and what's next; it's kept up to date there rather than
duplicated here.
