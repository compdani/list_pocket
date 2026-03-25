# Docker suite for development

**NOTE**: This exists only for local development. For production-style deployment, see the repository root `Dockerfile` and docs.

### Objective

The purpose of this Docker suite is to isolate dev dependencies (e.g. MailHog) and run the frontend/backend with the repo bind-mounted so you do not need a full image rebuild on every change.

List Pocket **does not use PostgreSQL**; all application data goes through **embedded PocketBase** (SQLite in `pb_data/`).

## Setting up a dev suite

Typical stack:

- MailHog (SMTP capture)
- Node.js frontend dev server
- Go backend

### Verify your config file

The config file at `dev/config.toml` (or the root `config.toml`) is used when running the stack. Adjust `[app]` and paths as needed.

### Commands

If your project still provides Make targets (or use `docker compose` directly from `dev/`):

```bash
docker compose -f dev/docker-compose.yml up
```

Visit the frontend URL (often `http://localhost:8080`) and the API on port `9000` as configured.

### Tear down

```bash
docker compose -f dev/docker-compose.yml down
```

### See local changes in action

- **Backend**: Rebuild or restart the Go process when you change server code.
- **Frontend**: With `npm run dev`, changes hot-reload when the app directory is mounted into the container.
