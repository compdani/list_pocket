# listpocket

listpocket is a self-hosted newsletter and mailing list manager. It started as a fork of [listmonk](https://listmonk.app) but has diverged significantly in both architecture and scope.

Maintained by Danilo Urrutia.

## What It Is

listpocket is intended for running and managing newsletters from your own infrastructure while keeping the application stack straightforward to develop and deploy.

## How It Differs from listmonk

While listpocket draws inspiration from listmonk, it is **not a drop-in replacement** and has been substantially reworked:

- **Frontend rewrite.** The admin UI is a complete rewrite in Vue 3 using Vuetify 4.0. None of the original frontend code remains.
- **PocketBase as the sole datastore.** Rather than supporting multiple SQL backends, listpocket uses [PocketBase](https://pocketbase.io) as its main and only database. This is a deliberate design choice — it simplifies deployment, embeds auth and storage, and removes the need for an external database server.
- **Go backend, restructured.** The backend retains a Go foundation but has been adapted to integrate directly with PocketBase rather than the original listmonk data layer.

## Project Status & Contributions

**This is a personal project.** A few things to be aware of before using or engaging with it:

- I am **not accepting** pull requests, contributions, issues, or change requests of any kind.
- The `main` branch may introduce breaking changes between versions without notice. Pin to a specific commit or release if stability matters for your deployment.
- You are completely free to **fork** the project and adapt it to your needs — that's what AGPLv3 is for.
- I may occasionally look at interesting forks. If I end up pulling ideas from one, I'll attribute the original author. This is a "maybe" — please don't fork with the expectation of being merged upstream.

If you need a managed, community-driven newsletter platform, [listmonk](https://listmonk.app) itself is an excellent option and likely a better fit.

## Run in Development

Before starting the app locally, make sure you have:

- Go
- Node.js and npm
- A local `config.toml` copied from `config.toml.sample`

Create your local config file:

```bash
cp config.toml.sample config.toml
```

Start the backend from the repository root:

```bash
go run ./cmd
```

Start the frontend dev server in a second terminal:

```bash
cd frontend
npm install
npm run dev
```

By default, the backend listens on `http://127.0.0.1:9000` and the frontend dev server runs on `http://127.0.0.1:8080`.

With the repo layout unchanged, the server also exposes **`/openapi.yaml`** and **`/swagger/`** (from `docs/swagger/`), and after `mkdocs build` in `docs/docs`, the manual at **`/docs/`**.

## Build

Build the frontend assets first, then compile the backend binary:

```bash
cd frontend
npm install
npm run build

cd ..
go build -o listpocket ./cmd
```

This produces the frontend bundle in `frontend/dist` and the backend binary at `./listpocket`.

## Tech Stack

- **Backend:** Go
- **Frontend:** Vue 3 + Vuetify 4.0
- **Database / Auth / Storage:** PocketBase

## License

listpocket is free and open source software licensed under **AGPLv3**.

## Credits

- [listmonk](https://listmonk.app) — original foundation and architectural inspiration.
- [Vuetify](https://vuetifyjs.com) — UI framework powering the rewritten admin frontend.
- [PocketBase](https://pocketbase.io) — embedded backend providing the database, auth, and storage layer.