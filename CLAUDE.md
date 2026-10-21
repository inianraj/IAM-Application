# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A JSON CRUD API over a single PostgreSQL table (`records`), written in Go (module name `crud`, Go 1.27) using only the standard library's `net/http` router plus the `pgx` driver via `database/sql`. There is no git repo, no tests, no Makefile, and no linter config yet.

## Commands

```powershell
docker compose up -d          # start Postgres 17 (reads DB_* vars from .env; DB_PASSWORD is required)
go run ./cmd/api              # run the API (default :8080, override with HTTP_ADDR)
go build ./...                # build
go vet ./...                  # static checks
go test ./...                 # run tests (none exist yet)
go test ./internal/service -run TestName   # single test, once tests exist
```

Migrations are plain SQL in [migrations/](migrations/) with no runner — the app does not create the schema. Apply them manually, e.g.:

```powershell
Get-Content migrations/001_create_records.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>
```

## Architecture

Layered, wired by hand in [cmd/api/main.go](cmd/api/main.go): `database.Open` → repository → service → handler → `http.ServeMux` routes under `/api/v1/records` (Go 1.22+ method/path patterns, `r.PathValue("id")`). The server shuts down gracefully on SIGINT/SIGTERM.

- **domain** — the entity is `domain.Task`, but it is stored in the `records` table and exposed as "record" in URLs and error messages. `Task.Validate()` trims fields *in place* (pointer receiver) and enforces a non-empty title of ≤200 chars, matching `VARCHAR(200)` in the migration. Sentinel errors live in [internal/domain/errors.go](internal/domain/errors.go).
- **repository** — raw SQL with `$n` placeholders. `scanTask` maps `sql.ErrNoRows` → `domain.ErrTaskNotFound`; `Delete` uses `RowsAffected` to do the same. `List` returns an empty slice (never nil) so the JSON is `[]`.
- **service** — depends on the `TaskStore` interface (not the concrete repository), validates IDs and entities, then delegates.
- **handler** — `decodeJSON` enforces a 1 MiB body limit, rejects unknown fields and trailing JSON values. `writeServiceError` maps sentinel errors to HTTP status (invalid → 400, not found → 404, everything else → generic 500 so DB details don't leak). New domain errors need a case here.
- **util** — `JSON` / `Error` response helpers; errors are always `{"error": "..."}`.

### Configuration quirks

- `config.Load()` parses `.env` into the process environment (without overriding existing vars), so it must run before `database.Open`.
- `config.Config.DatabaseURL` (built from `DATABASE_URL` or `DB_*`) is **not used** — `database.Open` reads `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` directly and requires all five to be set.

### Naming

The service and repository use `Task` names (`TaskService`, `TaskRepository`, `TaskStore`); the handler and routes use `Record` names. The service takes and returns `domain.Task` values, and the handler builds them from the request body.
