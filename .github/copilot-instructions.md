# listpocket

listpocket is a self-hosted newsletter and mailing list manager. The backend is Go (embedded PocketBase with native `RequestEvent` handlers), the frontend is Vue 3 + Vuetify, and SQLite (via PocketBase) is the database.

---

## Commands

### Backend
```bash
go run ./cmd                        # run dev server via pb.Start() → serve (port from config.toml app.address)
go run ./cmd serve --http=localhost:9000
go run ./cmd migrate up             # apply app migrations via migratecmd
go build -o listpocket ./cmd        # build binary

go test ./cmd -count=1              # test cmd package
go test ./internal/core -count=1    # test core package
go test ./cmd -run TestFoo -count=1 # run a single test
```

### Frontend
```bash
cd frontend
npm install
npm run dev          # dev server (port 8080)
npm run build        # production build → frontend/dist
npm run test:unit    # unit tests
```

### Config
Copy `config.toml.sample` → `config.toml` before first run. Env vars are prefixed `LISTPOCKET_` and merged into config at startup; double underscore maps to a nested key (`LISTPOCKET_app__lang` → `app.lang`).

---

## Architecture

### Request flow
The process starts with `pb.Start()` (PocketBase cobra CLI). Default command is `serve`; `app.address` from `config.toml` is bridged to `--http` when missing. App migrations and settings load in `OnBootstrap`; services wire and routes register in `OnServe`.

`/mailapi` handlers are native PocketBase `RequestEvent` handlers registered in `cmd/handlers.go` (`wrapEcho` is gone). Public subscription/tracking pages are registered separately (see `cmd/public.go`).

```
pb.Start() → serve
  → OnBootstrap (migrations, settings)
  → OnServe (wire App, register routes)
HTTP request
  → PocketBase router
  → RequestEvent handler (cmd/*.go)
      → core.Core method (internal/core/*.go)
        → PocketBase Record API and/or pbdb.DB (sqlx) for quarantined hot paths
```

API routes are prefixed `/mailapi/` and require PocketBase auth (`apis.RequireAuth()`).

### Layers
| Layer | Package | Responsibility |
|-------|---------|---------------|
| Handlers | `cmd/` | HTTP parsing, response serialisation, permission checks |
| Core | `internal/core/` | Business logic; Record API for CRUD; sqlx only on quarantined hot paths |
| DB wrapper | `internal/pbdb/` | Wraps PocketBase's `dbx.DB` in `sqlx` for raw SQL |
| Models | `models/` | Shared Go structs |
| Migrations | `internal/migrations/` | Numbered PocketBase migration files |

### Data access quarantine
Prefer the PocketBase Record API (`NewRecord` / `FindRecordById` / `Save` / `Delete` via `c.db.PocketBase()`) for new and migrated CRUD.

`sqlx` / `pbdb` is reserved for hot paths that need bulk SQL, joins, or ledger throughput:

- campaign ledger
- `manager_store` send loop
- subimporter
- subscriber query / export
- campaign analytics / dashboard aggregates
- timeline fan-in
- bulk subscription ops

Do not expand sqlx usage outside these quarantine zones without an explicit reason.

### Dual IDs / record-id-first API edges
Every record has both a **SQLite integer rowid** (internal / hot-path) and a **PocketBase string record ID** (`id` / `RecordID`, used in relation fields and API edges).

API request and response edges are **record-id-first**: path params and JSON `id` fields are PocketBase record ids (strings). Integer rowids may remain as deprecated aliases in a few write payloads (e.g. legacy `subscriber_ids`, int `media`) but new code should accept and emit string record ids.

Core helpers like `ResolveSubscriberIDs` and `ResolveListIDs` translate when a hot path still needs rowids. Prefer signatures such as `UpdateSubscriber(recordID string, ...)` so single-resource handlers do not need Resolve* at the edge.

---

## Key Conventions

### Permission model
Permissions are declared in `permissions.json` and enforced per-route via permission middleware helpers. The Super Admin role always receives all permissions (synced on startup in `main.go`). Passing multiple permission strings means the user needs **any one** of them.

### Core methods and errors
`internal/core` methods return HTTP errors (`echo.NewHTTPError` / `apperr.*`) on failure — callers can return these directly from handlers without wrapping.

### Migrations
Each migration is a numbered Go file in `internal/migrations/` that calls `m.Register(up, down)` from an `init()` function. Migrations are applied on bootstrap/serve (and via `migrate up`). `Automigrate` is disabled to prevent accidental file generation. The old `--install` / `--upgrade` flags are deprecated no-ops kept for listmonk automation compatibility.

### Timeline / unified events
`internal/core/timeline.go` (`GetUnifiedContactTimeline`) merges outbound campaign activity (campaign_send_ledger, campaign_views, link_clicks) with inbound events (inbound_sms_events, inbound_email_replies) into a single sorted `[]TimelineEvent`. When adding a new event source, follow the same pattern: query into a local slice → append to the shared events slice → let the unified sort handle ordering.

### Frontend
- Vue 3 + Vuetify, Vite. All API calls go through `frontend/src/api/index.js`.
- State management with Vuex (`frontend/src/store/`).
- i18n strings are in `i18n/` (backend) and loaded at runtime via `/mailapi/lang/{lang}`.
