# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A JSON CRUD API over PostgreSQL (`records`, `organisations`, plus `users` / `groups`), written in Go (module name `crud`, Go 1.27) using only the standard library's `net/http` router plus the `pgx` driver via `database/sql`. There is no Makefile and no linter config yet.

## Commands

```powershell
docker compose up -d          # start Postgres 17 (reads DB_* vars from .env; DB_PASSWORD is required)
go run ./cmd/api              # run the API (default :8080, override with HTTP_ADDR)
go run ./cmd/web              # run the web UI (default :3000) from the repository root; open http://localhost:3000
go build ./...                # build
go vet ./...                  # static checks
go test ./...                 # run tests (miniredis stands in for Redis; no Postgres needed)
go test ./internal/service -run TestName   # single test
swag init -g cmd/api/main.go -o docs --parseInternal   # regenerate Swagger docs (swag CLI v1.16.4)
```

Swagger UI is served at http://localhost:8080/swagger/index.html.

Migrations are plain SQL in [migrations/](migrations/) with no runner — the app does not create the schema. Apply each file in order manually, e.g.:

```powershell
Get-Content migrations/001_create_records.sql     | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>
Get-Content migrations/002_create_users_groups.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>
Get-Content migrations/003_create_organisations.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>
Get-Content migrations/004_groups_belong_to_organisation.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # deletes org-less groups; see the file header
Get-Content migrations/005_users_organisation.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>
Get-Content migrations/006_drop_users_password.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # drops users.password_hash
Get-Content migrations/007_records_user.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # records.user_id NOT NULL; existing records go to the lowest-id user
Get-Content migrations/008_users_password_signup_method.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # users.password_hash (nullable) + signup_method; existing users become 'legacy'
Get-Content migrations/009_users_email_signup_method.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # drops password_hash; signup_method becomes email|google|github; existing users become 'email'
Get-Content migrations/010_users_optional_organisation.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # users.organisation_id/group_id become nullable (both or neither); every organisation gets a 'member' group
Get-Content migrations/011_iam_permissions.sql | docker exec -i crud-api-postgres psql -U <DB_USER> -d <DB_NAME>   # permissions + group_permissions tables (seeded, existing groups keep today's access), records.organisation_id
```

## Architecture

Layered, wired by hand in [cmd/api/main.go](cmd/api/main.go): `database.Open` → repository → service → handler, then `router.New` builds the `http.ServeMux` (Go 1.22+ method/path patterns, `r.PathValue("id")`). The server shuts down gracefully on SIGINT/SIGTERM.

- **Routes** — each handler declares its own routes in a `Register(mux)` method (e.g. `RecordHandler.Register` in [record_handler.go](internal/handler/record_handler.go)), next to the `@Router` annotations they must match. [internal/router/router.go](internal/router/router.go) calls every `Register` and adds `/swagger/`; it also returns the final `http.Handler`, so shared middleware wraps it there. A new handler needs three edits: its `Register`, a field in `router.Handlers`, and construction in `main.go`.

- **domain** — the entity is `domain.Task`, but it is stored in the `records` table and exposed as "record" in URLs and error messages. `Task.Validate()` trims fields *in place* (pointer receiver) and enforces a non-empty title of ≤200 chars, matching `VARCHAR(200)` in the migration. Sentinel errors live in [internal/domain/errors.go](internal/domain/errors.go).
- **repository** — raw SQL with `$n` placeholders. `scanTask` maps `sql.ErrNoRows` → `domain.ErrTaskNotFound`; `Delete` uses `RowsAffected` to do the same. `ListByUser` returns an empty slice (never nil) so the JSON is `[]`.
- **service** — depends on the `TaskStore` interface (not the concrete repository), validates IDs and entities, then delegates.
- **handler** — `decodeJSON` enforces a 1 MiB body limit, rejects unknown fields and trailing JSON values. `writeServiceError` maps sentinel errors to HTTP status (invalid → 400, not found → 404, email/group already exists → 409, everything else → generic 500 so DB details don't leak). New domain errors need a case here.
- **util** — `JSON` / `Error` response helpers; errors are always `{"error": "..."}`.
- **docs/** (repo root) — generated by swaggo from the `// @...` annotations on `main()` and on each handler method; never edit by hand. Re-run `swag init` after changing routes, annotations, request structs, `domain.Task` / `domain.User` / `domain.Group`, or error responses, and commit the result. `--parseInternal` is required because the annotated types live under `internal/`. Keep the `github.com/swaggo/swag` version in `go.mod` matching the CLI version, or the generated `docs.go` may not compile.

### Users and groups

Every endpoint needs a login session except the public ones listed under "Authentication and authorization" below.

- **Create user** — `POST /api/v1/auth/users` `{name,email}` (served by `AuthHandler.CreateUser`) **needs a session and the `user:assign_group` permission** (admins; there is no public way to join an organisation) → 201 with the user `{id,name,email,group_id,organisation_id,signup_method,created_at,updated_at}`, added to the **caller's own organisation** in its basic `member` group (no `organisation_id` in the body; an unknown field is rejected). 401 without a session, 403 without the permission, 404 for a caller with no organisation, 400 invalid name/email, 409 if the email exists (the `users.email` UNIQUE constraint, with the email lower-cased, is the source of truth). There is no password: the user then signs in with `POST /auth/login {email}`. `UserService.Create` validates and sets `SignupMethod = email`. An organisation's first admin is not created here but together with the organisation (see Organisations). Possible refinement: a dedicated `user:create` permission instead of reusing `user:assign_group`.
- **Signup method** (migrations 008/009): `users.signup_method` is `email` (created through `POST /auth/users`, signs in with just the email), `google` or `github` (future provider logins). A CHECK constraint (`users_signup_method_check`) allows only those values; there is no password column any more (009 dropped `password_hash` and turned every `password`/`legacy` user into `email`). Adding a provider means one more value in that constraint plus a `SignupMethod*` constant and its own login endpoint calling `domain.CheckSignInMethod`. Email is globally unique, so one email can't exist with two methods (no account linking yet).
- **Groups CRUD** — `POST /api/v1/groups` `{organisation_id,name[,permissions]}` → 201 `{id,organisation_id,name}` (`permissions` are keys from `GET /permissions`; omitted = the member set; see "Permissions (IAM)"); `GET /api/v1/groups?organisation_id=N` (the parameter is **required**: only that organisation's groups are listed; 400 if missing/invalid, 404 if the organisation doesn't exist, `[]` if it has no groups); `GET/PUT/DELETE /api/v1/groups/{id}` (PUT takes `{organisation_id,name}`, both required, so it can move a group to another org). Errors: 400 invalid/missing `organisation_id` or name, 404 unknown group or organisation, 409 name already used **in that organisation**. `DELETE` returns 204, or 409 `group still has users` (the `users.group_id` FK violation maps to `ErrGroupInUse`).
- Group names are lower-cased, must match `^[a-z][a-z0-9_-]*$`, and are unique **per organisation** (`UNIQUE (organisation_id, name)`), so the same name can exist in several organisations. `organisation_id` is `NOT NULL`, with no default.
- `GET /api/v1/groups/{id}/users` → 200 `[{id,name,email}, ...]` (oldest user first; `[]` for an empty group); 404 if the group doesn't exist. It takes the group **id**, not the name, because names are no longer globally unique. One `LEFT JOIN` query tells an empty group apart from a missing one.
- **Other user endpoints** — `GET /api/v1/users/{id}` → 200 user. `PUT /api/v1/users/{id}` `{name}` → 200 user (**name only**: the email is the login identity and never changes, an `email` field is rejected as unknown; the group is never changed here). `DELETE /api/v1/users/{id}` → 204, or 409 `user still has records` (the `records.user_id` FK violation maps to `ErrUserInUse`; delete the user's records first). Unknown user → 404, email taken → 409. There is no list-users endpoint. The user JSON is `{id,name,email,group_id,organisation_id,signup_method,created_at,updated_at}`.
- `users.organisation_id` and `users.group_id` are nullable in the database (migration 010) and either both set or both NULL (`users_org_group_both_or_neither`), but **the API no longer creates users without an organisation**; older rows may still have none (in Go both are `0`, `scanUser` reads `sql.NullInt64`, JSON omits them) and can only read themselves and manage their own records. When both are set, a composite foreign key `users (group_id, organisation_id) → groups (id, organisation_id)` makes the **database** guarantee the group belongs to the user's organisation (PostgreSQL skips that FK when a column is NULL, hence the CHECK). A user stays in one organisation for life (update never changes it). The repository's checks exist to give the right error; the FK is the safety net.
- `PUT /api/v1/users/{id}/group` `{group_id}` → 200 `{user_id,group_id}`. **The group must belong to the organisation stored on the user**: 400 `group does not belong to the user's organisation` otherwise; 404 for an unknown user or group. `UserRepository.Create` and `AssignGroup` do the check and the write in one SQL statement; only when it affects zero rows do they run a second query, to pick the right error.
- Because of that FK: a group that has users can't be moved to another organisation (`PUT /groups/{id}` → 409 `group still has users`, mapped in `scanGroup` by constraint name; a missing target organisation is still 404), and deleting a group or organisation that has users is 409.
- Validation is regex-based in [internal/domain/user_validation.go](internal/domain/user_validation.go): email shape and a blocklist of personal-provider domain patterns (gmail, yahoo.*, hotmail.*, ... — add patterns there).
- **Groups every organisation has** (`domain.AdminGroupName` = `admin`, `domain.MemberGroupName` = `member`, in [internal/domain/group.go](internal/domain/group.go)): `admin` has every permission, `member` is the basic group of users created with `POST /auth/users`. Both are created with the organisation (see Organisations) together with their permissions, and **cannot be renamed, deleted or given other permissions** (`ErrBuiltinGroup`, 400), so an organisation can't lock itself out. Authorization no longer looks at group *names*; it looks at the group's permissions.
- Layers mirror the records code: `UserHandler` / `GroupHandler` → `UserService` / `GroupService` (`UserStore` / `GroupStore` interfaces) → `UserRepository` / `GroupRepository`. `handler.parsePathID` takes the error message; `parseID` is the "record" variant.

### Records belong to a user

`records.user_id` is `NOT NULL` and references `users(id)` (migration 007; the one existing record was given to the lowest-id user).

- `POST /api/v1/records` `{user_id,title[,description]}` → 201 record (now includes `user_id`); 400 if `user_id` is missing/≤0, 404 `user not found` if the user doesn't exist (the FK violation maps to `ErrUserNotFound` in `scanTask`).
- Each record also stores its owner's `organisation_id` (migration 011; NULL/omitted for legacy owners without an organisation), stamped by `TaskService.Create` and kept consistent with the owner by the composite FK `(user_id, organisation_id) → users`. Permission checks use the record's own `user_id` and `organisation_id`.
- `GET /api/v1/records` has three modes: `?user_id=N` (that user's records; 400 if invalid, 404 if the user doesn't exist, `[]` if none; one `LEFT JOIN` query tells an empty list from a missing user), `?scope=org` (every record of the caller's organisation, needs `record:read:org`, cached as `records:org:{id}`), and neither (the caller's own records).
- `GET`/`PUT`/`DELETE /api/v1/records/{id}` are unchanged. `PUT` takes only `{title,description}` (`recordRequest`); the owner can't be changed, and a `user_id` in the body is rejected as an unknown field. Create uses a separate `createRecordRequest`.

### Organisations

A standalone CRUD over the `organisations` table (`id`, `name`). **Get, update and delete take no id: they act on the caller's own organisation**, read from the signed-in user the auth middleware loaded for the request (`auth.Principal.User.OrganisationID`, i.e. live data, not the copy in the Redis session), so there is no id to forge: `GET /api/v1/organisation` (any member; 404 for a user without an organisation), `PUT /api/v1/organisation` `{name}` (admin; 403 for members; 409 name taken), `DELETE /api/v1/organisation` `{"confirm_name":"<exact name>"}` (admin; 400 if the name doesn't match; 204). The public list is `GET /api/v1/organisations` (needed to find an id for `POST /auth/users`). **Creating** an organisation is `POST /api/v1/auth/organisations` `{name,admin_name,admin_email}` (public, served by `AuthHandler.CreateOrganisation`) → 201 `{id,name,admin:{user}}`: the organisation and its **first admin user are created together**; the email must not be registered yet (409 `email already registered`), the admin name/email follow the user rules (400, work email only). Duplicate name → 409. The service methods are `GetCurrent` / `UpdateCurrent` / `DeleteCurrent`.

- Names are trimmed, required, ≤100 characters, and unique **ignoring case** via a unique index on `LOWER(name)` (the stored name keeps the case given). Updating an org to its own name in a different case is allowed.
- The repository maps both `sql.ErrNoRows` (→ `ErrOrganisationNotFound`) and unique violations (→ `ErrOrganisationExists`) inside `scanOrganisation`, so `Create`/`Update` get the 409 for free. The unique-violation check reuses `pgCode` / `uniqueViolation` from `user_repository.go`.
- **Every organisation gets an `admin` and a `member` group and its first admin user**: `OrganisationRepository.Create(ctx, org, admin)` inserts the org, both groups and the admin user (in `admin`, signup method `email`) in one transaction, so a duplicate org name or a taken admin email (409) leaves nothing behind. This is the usual tenant-bootstrap pattern: the tenant and its first admin are created atomically, and everyone else is added afterwards. `CachedOrganisationStore.Create` drops `orgs` and `groups:org:{id}` (the user and groups are new, so nothing else can be cached). Not done: the admin created here gets no `user.signed_up` welcome-email event (only `EventUserStore.Create` publishes it).
- **Delete is a hard, cascading delete** (`OrganisationRepository.Delete`, one transaction): the records of the organisation's users, then its users (the caller included), then the organisation, whose groups go with it (`groups.organisation_id` is `ON DELETE CASCADE`). It returns the removed user and record ids (`domain.DeletedOrganisation`) for cache invalidation. A user created in the organisation while the delete runs makes the last step fail on the `users.group_id` FK: 409 `ErrOrganisationInUse`, nothing removed, retry. Sessions of the deleted users need no cleanup: the middleware answers 401 and clears the cookie for a deleted user, and the Redis keys expire by TTL. Not done (industry norm for tenants): soft delete with a retention window, re-authentication and an audit log; the typed `confirm_name` is the only safeguard.

### Configuration quirks

- `config.Load()` parses `.env` into the process environment (without overriding existing vars), so it must run before `database.Open`.
- `config.Config.DatabaseURL` (built from `DATABASE_URL` or `DB_*`) is **not used** — `database.Open` reads `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` directly and requires all five to be set.

### Naming

The service and repository use `Task` names (`TaskService`, `TaskRepository`, `TaskStore`); the handler and routes use `Record` names. The service takes and returns `domain.Task` values, and the handler builds them from the request body.

### Redis cache (records, users, groups, organisations)

Cache-aside layer over Redis, configured by `REDIS_ADDR` (e.g. `localhost:6379`; **required**, because login sessions live in Redis too and the API exits at startup without it). Also `REDIS_PASSWORD`, `REDIS_DB`, `CACHE_TTL_SECONDS` (default 60). Redis runs as `crud-api-redis` in `compose.yaml` (plain `redis:7`); inspect with `docker exec -it crud-api-redis redis-cli`.

- [internal/cache/task_store.go](internal/cache/task_store.go) `CachedTaskStore` decorates `TaskRepository` and is wired in `main.go` (service and handlers are unchanged). Redis errors at runtime are logged and treated as cache misses (reads fall back to Postgres), but sessions fail closed (see Authentication).
- Keys (JSON strings with TTL, prefix `crud:v1:`): `record:{id}`, `records:user:{userID}`, `records:org:{orgID}`. Only successful reads are cached (never not-found).
- Invalidation after a successful DB write: create → user list; update → record + user list; delete → record + user list (owner read from Postgres first). TTL is the backstop.
- Users: `CachedUserStore` ([user_store.go](internal/cache/user_store.go)) caches `GetByID` (`user:{id}`). Writes drop it **and the affected `group:{gid}:users` member lists** (create → new group; update/delete → current group; assign → old and new group; the old group is read from Postgres first). Shared Redis helpers are in [json_cache.go](internal/cache/json_cache.go).
- Groups: `CachedGroupStore` caches `group:{id}`, `groups:org:{orgID}`, `group:{id}:users` and `perms:group:{id}` (the permissions authorization reads on every request; `SetPermissions` and group delete drop it). Create → org list; update → item + old and new org lists (a group can move org); delete → item, members, org list.
- Organisations: `CachedOrganisationStore` caches `org:{id}` and `orgs` (full list). Create/update drop them; delete also drops `groups:org:{id}`, every cascade-deleted group's `group:{gid}` / `group:{gid}:users` (ids listed from the group repository before deleting) and, for every removed user and record, `user:{id}`, `records:user:{id}` and `record:{id}` (ids returned by the repository). Any new write path that changes a cached group, user or org must invalidate the keys above.
- `go test ./internal/cache` uses miniredis.

### Frontend (web/)

A plain HTML + JavaScript UI (no framework, no build step, no npm), served by its **own small server**: `go run ./cmd/web` (default `:3000`, flags `-addr`, `-dir`) and opened at http://localhost:3000. It talks to the API at `API_BASE` in [web/config.js](web/config.js) (`http://localhost:8080/api/v1`).

- **Cross-origin setup**: the API must list the UI's origin, exactly as the browser shows it, in `ALLOWED_ORIGINS` (`.env` has `ALLOWED_ORIGINS=http://localhost:3000`; `127.0.0.1` is a different origin). `auth.OriginGuard` then answers the preflight and sends `Access-Control-Allow-Origin` + `Allow-Credentials`; every `fetch` uses `credentials: "include"`. `SameSite=Strict` still works because `localhost:3000` and `localhost:8080` are the same *site* (ports are ignored). In production the UI and API must share a site (e.g. `app.example.com` / `api.example.com`) and `COOKIE_SECURE=true`.
- **Files**: `index.html`, `style.css`, `app.js` (hash router, nav built from permissions, logout, 401 handling), `api.js` (one `request()` helper, `ApiError`), `state.js` (logged-in user, permissions, `can()` / `canOn()`), `dom.js` (`h()` element builder; server data is only ever added as text, never HTML), and one file per screen in `views/` (`auth`, `records`, `groups`, `people`, `organisation`).
- **Identity**: there is no `GET /auth/me`. After login the user id is kept in `localStorage`; on page load the UI calls `GET /users/{id}` (a user can always read themselves): 200 restores the session, 401 clears it. The permissions come from `GET /groups/{group_id}/permissions`. The UI only hides what would return 403; the API enforces everything.
- **Screens**: login and create organisation (public; users are added by an admin on the People screen, there is no self-service join); records (own, or the whole organisation with `record:read:org`), groups (create with permission checkboxes from `GET /permissions`, edit permissions, rename, delete; built-in groups read-only), people (move/rename/delete members, add a member; needs `GET /groups/{id}/users` to include the user `id`), organisation (rename, delete with the typed name).

### RabbitMQ events and the worker (signup welcome email)

Optional, enabled by `RABBITMQ_URL` (e.g. `amqp://guest:guest@localhost:5672/`; empty = off). `compose.yaml` runs `crud-api-rabbitmq` (management UI http://localhost:15672, guest/guest) and `crud-api-mailpit` (SMTP `localhost:1025`, inbox UI http://localhost:8025). Config: `RABBITMQ_URL`, `SMTP_ADDR` (default `localhost:1025`), `MAIL_FROM` (default `no-reply@crud.local`).

- **Publisher** — [internal/events/user_store.go](internal/events/user_store.go) `EventUserStore` wraps `service.UserStore` (outermost, after the cache) and publishes `user.signed_up` after a successful `Create`. Publishing uses broker confirms with a 2 s timeout; a failure is logged and **never fails the signup** (the welcome email is then lost — simple publish-after-commit, no outbox). If the broker is down at API start the API logs a warning and runs without events.
- **Event** — JSON envelope `{type,id,occurred_at,data}`; `data` = `{user_id,name,email}`. Exchange `crud.events` (topic), routing key `user.signed_up`.
- **Topology** ([topology.go](internal/events/topology.go), declared idempotently by API and worker): queue `email.welcome` → on failure the worker republishes to `email.welcome.retry` (30 s TTL, dead-letters back to `email.welcome`) with an `x-attempt` header; after 3 attempts, or on a permanent failure, to `email.welcome.dlq`.
- **Worker** — `go run ./cmd/worker` (separate process; needs RabbitMQ and an SMTP server). [internal/worker/welcome_email.go](internal/worker/welcome_email.go) decides the outcome (ack / retry / dead-letter) and is unit-tested; [consumer.go](internal/worker/consumer.go) applies it with manual acks. SMTP **5xx replies and invalid messages are permanent → DLQ at once**; 4xx, timeouts and connection errors are retried (`mail.IsPermanent`). Malformed events and CR/LF in name/email go to the DLQ. Delivery is at-least-once, so a rare duplicate email is possible.
- The SMTP sender ([internal/mail/smtp.go](internal/mail/smtp.go)) has no auth or TLS — local relay/Mailpit only.

### Authentication and authorization

**Login** — `POST /api/v1/auth/login` `{email}` (no password, **no proof of email ownership**, see the known limitation below) → 200 user and a `session` cookie (`HttpOnly`, `SameSite=Strict`, `Max-Age` = session TTL, `Secure` unless `COOKIE_SECURE=false`; `.env` sets it false for local http). `POST /api/v1/auth/logout` → 204 (clears the cookie, deletes the session). [AuthService](internal/service/auth_service.go) checks, in order: rate limit (429, `Retry-After: 900`), user lookup by email (unknown → 401 `invalid email`, counted as a failure), then the **sign-in method** (`domain.CheckSignInMethod`: an account only signs in with the method it was created with → 403 `wrong sign-in method: this account was created with google, sign in with google`). A future `POST /auth/google` calls `CheckSignInMethod` with `SignupMethodGoogle`.

**What is stored (all Redis, prefix `crud:v1:`)**
- `session:{sha256(token)}` → JSON `{user_id,organisation_id,created_at,ip,user_agent}`, TTL `SESSION_TTL_HOURS` (default 24, absolute). The browser holds the raw random 256-bit token; only its hash is a key, so a Redis leak yields no usable tokens. Sessions prove identity only: role and group are read fresh per request.
- `login_fail:{sha256(email)}:{ip}` → counter; 5 failures (unknown emails) block that email+IP for 15 minutes from the first failure; a successful login deletes it. The IP is `r.RemoteAddr` (no proxy-header trust: behind a proxy every client shares one IP).
- Nothing new in Postgres. `AuthService` looks users up with `UserRepository.GetByEmail`, injected directly so it bypasses the cache decorators (the cache has no email key).

**Every request** — [auth.Middleware](internal/auth/middleware.go) (default-deny; wrapped in `router.New` through `Handlers.Middleware`, after `auth.OriginGuard`): cookie → `session.Store.Get` → load the user from the cached `UserStore` → `auth.Principal` in the request context (`auth.FromContext`). Missing/unknown/expired session or a deleted user → 401 (and the cookie is cleared); Redis or user-store failure → **503, never let through**. Public routes are `router.PublicRoutes`: `POST /auth/login`, `POST /auth/organisations` (creates an organisation with its first admin), `GET /organisations` (still public although nothing needs it any more; it lists org names and ids) and `GET /swagger/`. **A new route is protected unless added to that list.**

**CSRF/CORS** — `SameSite=Strict` plus `auth.OriginGuard`: a POST/PUT/PATCH/DELETE with an `Origin` header must be this host or listed in `ALLOWED_ORIGINS` (comma-separated), else 403. Listed origins also get CORS headers with credentials (preflight answered); with the list empty the API is same-origin only.

**Authorization — permissions (IAM)** ([internal/authz](internal/authz/authz.go), called from the **services**, so a new handler can't skip it). A user is in one group, a **group is given permissions** (no roles layer), and every endpoint requires one. Another organisation's resources return the resource's own 404 (no existence leak); in-org but not allowed returns 403 `forbidden`.

- **The `permissions` table is the only list of permissions** (21 rows, seeded by migration 011; `GET /permissions` reads it). The code holds only the *names its checks require*, as constants in [internal/domain/permission.go](internal/domain/permission.go) (`PermUserRead`, `ScopedKey(resource, action, scope)` ...). Form `resource:action[:scope]`: `record:create|read|update|delete` × `:own|:org`, `user:read`, `user:update:own|org` (name only), `user:delete`, `user:assign_group`, `group:create|read|update|delete`, `group:manage_permissions`, `organisation:read|update|delete`. `own` = the caller's own item, `org` = any item in their organisation; resources with no owner have no scope. `internal/domain/permission_test.go` reads migration 011 and fails if a permission the code checks is not seeded (or the seed has one the code never checks).
- **Adding a permission** = (1) a constant in `permission.go`, (2) the check in the service, (3) a **new migration** that inserts the row and gives it to the groups that should have it, e.g. `INSERT INTO permissions (key, resource, action, scope, description) VALUES (...);` then `INSERT INTO group_permissions (group_id, permission) SELECT g.id, '<key>' FROM groups g WHERE g.name = 'admin';` (existing admin groups do **not** get new permissions automatically; add the same for `member` or custom groups only if intended), (4) update the seed count in `permission_test.go`. There is no startup sync.
- **Tables** (migration 011) — `permissions(key, resource, action, scope, description)`; `group_permissions(group_id, permission → permissions.key)` (the database is the validator: an unknown key violates `group_permissions_permission_fkey`, mapped to `ErrInvalidGroup` 400 and the transaction rolls back; both FKs cascade). `domain.UniquePermissions` removes duplicates before storing. Built-in groups, created with every organisation by `OrganisationRepository.Create`: `admin` gets **every row of the permissions table** (`INSERT ... SELECT key FROM permissions`), `member` = `domain.MemberPermissions()` (`record:*:own`, `user:read`, `user:update:own`, `group:read`, `organisation:read`). New custom groups get the permissions in the request or the member set; the migration gave existing `admin` groups everything and every other group the member set.
- **Engine** — `Require(ctx, perm, orgID, notFound)` (other organisation or no organisation → `notFound`, missing permission → `ErrForbidden`), `RequireScoped(ctx, resource, action, ownerID, ownerOrgID, notFound)` (`:org` permission on anything in the caller's organisation, `:own` only when `ownerID == caller.ID`), `RequireScopedForUser` / `RequireForUser` (load the target user first), `RequireSelfOr` (a user can always read themselves), `Has`. Permissions are read per request through `CachedGroupStore.Permissions` (`perms:group:{gid}`, invalidated by `SetPermissions`, group delete and organisation delete), so a changed permission applies on the next request. A legacy caller without an organisation has no group and no permissions and may only act on their own resources.
- **Managing permissions** — `GET /permissions` (needs `group:manage_permissions`, i.e. admins only: the catalogue with descriptions), `GET /groups/{id}/permissions` (`group:read`), `PUT /groups/{id}/permissions` `{permissions:[...]}` (`group:manage_permissions`; replaces the set; unknown key → 400; built-in groups → 400), `POST /groups` with `permissions` needs `group:create` and `group:manage_permissions`.

| Route | Permission |
|---|---|
| `GET /organisation` | `organisation:read` |
| `PUT /organisation` | `organisation:update` |
| `DELETE /organisation` | `organisation:delete` (+ `confirm_name`; removes the org, its users and their records) |
| `POST /groups` | `group:create` (+ `group:manage_permissions` when `permissions` is given), `organisation_id` = own org |
| `GET /groups?organisation_id`, `GET /groups/{id}`, `GET /groups/{id}/users`, `GET /groups/{id}/permissions` | `group:read` |
| `PUT /groups/{id}` | `group:update` (can't move a group to another org; built-in groups can't be renamed) |
| `DELETE /groups/{id}` | `group:delete` (built-in groups can't be deleted) |
| `GET /users/{id}` | yourself, or `user:read` |
| `PUT /users/{id}` | `user:update:own` (yourself) / `user:update:org` (others) |
| `DELETE /users/{id}` | `user:delete` |
| `PUT /users/{id}/group` | `user:assign_group` |
| `POST /auth/users` | `user:assign_group` (adds a member to your own organisation) |
| `POST /records` | `record:create:own` (`user_id` = yourself) / `record:create:org` (for others) |
| `GET /records?user_id=N`, `GET /records` (own) | `record:read:own` / `record:read:org` |
| `GET /records?scope=org` | `record:read:org` |
| `GET /records/{id}` | `record:read:own` for your own, `record:read:org` for others |
| `PUT /records/{id}`, `DELETE /records/{id}` | `record:update` / `record:delete`, `:own` or `:org` the same way |

New errors (`domain/errors.go`, mapped in `writeServiceError`): `ErrInvalidCredentials`/`ErrUnauthenticated` 401, `ErrWrongSignupMethod`/`ErrForbidden` 403, `ErrTooManyAttempts` 429, `ErrBuiltinGroup` 400, `ErrUnavailable` 503 (fixed message; never echoes Redis errors).

**Known limitation (accepted by the project owner, development only):** there is no secret anywhere. `POST /auth/organisations` is public and `POST /auth/login` needs only an email, so **anyone can create an organisation with any (new) email as its admin, and anyone who knows a user's email can sign in as that user** (admins included). Users can no longer join an organisation on their own: `POST /auth/users` needs an admin's session. `GET /organisations` still lists every organisation publicly. What a production IAM adds: email verification before the admin is active, and a membership table so a person can belong to several organisations. Planned fix for login: emailed one-time code (`POST /auth/login` sends it, `POST /auth/login/verify {email,code}` creates the session; reuse the RabbitMQ worker + Mailpit and the Redis helpers). Other gaps: there is no "log out everywhere"; Redis holds sessions in memory (a container restart keeps them only through Redis's default RDB snapshot; recreating the container logs everyone out).
