# listpocket

listpocket is a self-hosted newsletter, messaging, and contact platform. It started as a fork of [listmonk](https://listmonk.app) but has diverged in architecture and scope: campaigns and lists remain the core, with an inbox, CRM boards, workflows, an AI agent, WhatsApp, and a plugin host on top.

Maintained by Danilo Urrutia.

## What It Is

A single Go binary plus a Vue 3 admin UI. Persistence, auth, and file storage are [PocketBase](https://pocketbase.io) (SQLite under `pb_data/`). There is no external database server.

The Go server uses PocketBase v0.41.1 from `ganigeorgiev.com/pocketbase/pocketbase`.
Its companion `dbx` and validation modules also use the new host. The Docker image
builds this application from source; it does not download a prebuilt PocketBase executable.

The admin UI lives at `/admin/`. Public subscription, tracking, and archive URLs are served from the same process.

## Capabilities

**Audience**
- Subscribers, lists, import, bounce processing, and public subscription forms
- Contact pages with a unified activity timeline
- Boards and tasks for pipeline work around those contacts

**Messaging**
- Email, SMS, and WhatsApp campaigns (Meta Cloud API or Whapi.Cloud)
- Rich text, visual, HTML, and MJML editors, plus reusable templates
- Media library (PocketBase files, used from the editors and the assistant)
- Transactional messages, public archives, analytics, and open/click tracking
- Inbox for inbound email and WhatsApp replies

**Automation and AI**
- Visual workflows: triggers, branches, loops, reviews, waits, tools, JavaScript code nodes, and set nodes for variables
- Agent page and assistant side panel: read with your permissions, propose writes for you to review
- Workflow AI steps using the same provider as Settings → AI (OpenAI-compatible API key, or ChatGPT plan sign-in)

**Admin**
- Users, roles, list-scoped permissions, 2FA, and optional OIDC
- Super-admin plugins: new channels, webhooks, admin UI, and workflow tools without a rebuild
- Light / dark / system theme

User-facing documentation is the Zensical manual (build it, then open `/docs/`). The OpenAPI spec is at `/openapi.yaml` and Swagger UI at `/swagger/`.

## How It Differs from listmonk

listpocket is **not a drop-in replacement**:

- **Frontend rewrite.** Vue 3 and Vuetify 3. None of the original frontend remains.
- **PocketBase only.** No PostgreSQL or other SQL backends. Auth, collections, realtime, and uploads go through PocketBase.
- **Go backend, restructured.** HTTP is PocketBase `RequestEvent` handlers (`/mailapi/`), not the original listmonk Echo stack. Integer rowids still exist internally; API edges use PocketBase record IDs.
- **Beyond newsletters.** Inbox, boards, tasks, workflows, the agent, WhatsApp, and plugins are not in upstream listmonk.

Upstream listmonk guides that assume `LISTMONK_*` or a `[db]` block do not apply here. Environment variables are `LISTPOCKET_*`.

## Project Status & Contributions

**This is a personal project.** A few things to be aware of before using or engaging with it:

- I am **not accepting** pull requests, contributions, issues, or change requests of any kind.
- The `main` branch may introduce breaking changes between versions without notice. Pin to a specific commit or release if stability matters for your deployment.
- You are completely free to **fork** the project and adapt it to your needs — that's what AGPLv3 is for.
- I may occasionally look at interesting forks. If I end up pulling ideas from one, I'll attribute the original author. This is a "maybe" — please don't fork with the expectation of being merged upstream.

If you need a managed, community-driven newsletter platform, [listmonk](https://listmonk.app) itself is an excellent option and likely a better fit.

## Run in Development

You need Go (see `go.mod`), Node.js and npm, and a local `config.toml`:

```bash
cp config.toml.sample config.toml
```

Backend, from the repository root:

```bash
go run ./cmd
```

Frontend, in a second terminal:

```bash
cd frontend
npm install
npm run dev
```

By default the backend is `http://127.0.0.1:9000` and the Vite app is `http://127.0.0.1:8080` (it proxies `/api`, `/mailapi`, and related paths to the backend).

Optional:

- `LISTPOCKET_AI_API_KEY`, `LISTPOCKET_AI_BASE_URL`, and `LISTPOCKET_AI_MODEL` for the assistant, Agent page, and workflow AI steps
- `zensical build` in `docs/docs` so `/docs/` is available while the backend is running
- `go run ./cmd migrate up` to apply migrations without serving (they also run on startup)

## Build

Frontend first, then the binary:

```bash
cd frontend
npm install
npm run build

cd ..
go build -o listpocket ./cmd
```

That writes `frontend/dist` and `./listpocket`. The binary embeds templates, i18n, permissions, and the sample config. Serve the admin UI from `frontend/dist` on disk. Run with `config.toml` and optional `LISTPOCKET_*` variables (`LISTPOCKET_app__address` maps to `app.address`).

Docker (multi-stage: frontend, Zensical, Go):

```bash
docker build -t listpocket .
docker run --rm -p 9000:9000 -v listpocket-data:/app/pb_data listpocket
```

The image listens on `0.0.0.0:9000` and stores PocketBase data in `/app/pb_data`.

## Tech Stack

- **Backend:** Go, embedded PocketBase
- **Frontend:** Vue 3, Vuetify 3, Vite
- **Database / auth / files:** PocketBase (SQLite)
- **Editors and canvas:** TinyMCE, GrapesJS (MJML), Vue Flow (workflows)

## License

listpocket is free and open source software licensed under **AGPLv3**.

## Credits

- [listmonk](https://listmonk.app) — original foundation and architectural inspiration.
- [Vuetify](https://vuetifyjs.com) — UI framework for the admin frontend.
- [PocketBase](https://pocketbase.io) — embedded database, auth, and storage.
