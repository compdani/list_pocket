# listpocket – Copilot Instructions

listpocket is a self-hosted newsletter and mailing list manager. The backend is Go (Echo, PocketBase), the frontend is Vue 3 + Vuetify, and SQLite (via PocketBase) is the database.

---

## Commands

### Backend
```bash
go run ./cmd                        # run dev server (port 9000)
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
All HTTP routes are registered in `cmd/handlers.go` against PocketBase's router. Every handler is an Echo `HandlerFunc` bridged to PocketBase via `wrapEcho()` (also in `cmd/handlers.go`). `wrapEcho` constructs a temporary Echo context from the PocketBase `RequestEvent`, copies path params, resolves the authenticated user, and sets it on the context.

```
HTTP request
  → PocketBase router (pb.RootCmd)
    → wrapEcho() adaptor
      → Echo handler (cmd/*.go)
        → core.Core method (internal/core/*.go)
          → pbdb.DB / PocketBase API
```

API routes are prefixed `/mailapi/` and require PocketBase auth (`apis.RequireAuth()`). Public subscription/tracking pages are registered separately (see `cmd/public.go`).

### Layers
| Layer | Package | Responsibility |
|-------|---------|---------------|
| Handlers | `cmd/` | HTTP parsing, response serialisation, permission checks |
| Core | `internal/core/` | Business logic, DB queries, returns `echo.HTTPError` |
| DB wrapper | `internal/pbdb/` | Wraps PocketBase's `dbx.DB` in `sqlx` for raw SQL |
| Models | `models/` | Shared Go structs |
| Migrations | `internal/migrations/` | Numbered PocketBase migration files |

### PocketBase integration
PocketBase manages auth, the SQLite database, real-time events, file storage, and schema migrations. The app uses two complementary access patterns:

- **`pbdb.DB`** (sqlx wrapper) – raw SQL for complex joins and bulk operations using integer rowids.
- **`pb.FindCollectionByNameOrId` / `pb.Save`** – PocketBase ORM API for record-level operations (used in migrations and inbound event creation).

### Dual IDs
Every record has both a **SQLite integer rowid** (`id` column, used in raw SQL) and a **PocketBase string record ID** (`record_id` / `pb.Id`, used in relation fields). Core helpers like `ResolveSubscriberIDs` and `SQLiteListRecordIDs` translate between them. Always verify which type a function expects before passing IDs into it.

---

## Key Conventions

### Permission model
Permissions are declared in `permissions.json` and enforced per-route via `a.auth.Perm(handler, "resource:action")`. The Super Admin role always receives all permissions (synced on startup in `main.go`). Passing multiple permission strings to `pm()` means the user needs **any one** of them.

### Core methods and errors
`internal/core` methods return `echo.NewHTTPError(status, i18n.T("key"))` on failure — callers can return these directly from handlers without wrapping.

### Migrations
Each migration is a numbered Go file in `internal/migrations/` that calls `m.Register(up, down)` from an `init()` function. Migrations are applied automatically at startup. `Automigrate` is disabled in production to prevent accidental file generation.

### Timeline / unified events
`internal/core/timeline.go` (`GetUnifiedContactTimeline`) merges outbound campaign activity (campaign_send_ledger, campaign_views, link_clicks) with inbound events (inbound_sms_events, inbound_email_replies) into a single sorted `[]TimelineEvent`. When adding a new event source, follow the same pattern: query into a local slice → append to the shared events slice → let the unified sort handle ordering.

### Frontend
- Vue 3 + Vuetify, Vite. All API calls go through `frontend/src/api/index.js`.
- State management with Vuex (`frontend/src/store/`).
- i18n strings are in `i18n/` (backend) and loaded at runtime via `/mailapi/lang/{lang}`.
