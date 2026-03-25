# Developer setup

The app has two parts: the Go backend (with embedded PocketBase) and the Vue 3 frontend. In development they run as separate processes.

### Prerequisites

- Go (see `go.mod` for the toolchain version)
- Node.js and npm (for the frontend)
- PostgreSQL — create an empty database and point `[db]` in `config.toml` at it

### First-time setup

1. Clone this repository (it uses Go modules; clone outside `GOPATH/src` if you still use legacy layouts).
2. Copy `config.toml.sample` to `config.toml` and set database credentials.
3. From the repo root, build the frontend and run the backend (see below).

[MailHog](https://github.com/mailhog/MailHog) is a useful mock SMTP server with a web UI for local e-mail testing.

### Running locally

1. **Backend** — from the repository root:

   ```bash
   go run ./cmd
   ```

   By default the app listens on `http://127.0.0.1:9000`.

2. **Frontend** — in a second terminal:

   ```bash
   cd frontend
   npm install
   npm run dev
   ```

   The dev server (default `http://127.0.0.1:8080`) proxies `/api/*` to the backend.

See the [frontend README](https://github.com/compdani/list_pocket/blob/master/frontend/README.md) for structure and conventions.

### Production build

Build the frontend, then compile the binary:

```bash
cd frontend && npm install && npm run build
cd .. && go build -o listpocket ./cmd
```

This embeds the built assets into the binary. Run `./listpocket` with your `config.toml` (and optional `LISTPOCKET_*` environment variables).
