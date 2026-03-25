# Installation

List Pocket ships as source in this repository. It is a **Go** application built around **embedded PocketBase** for all persistence, authentication, and realtime features. Data is stored in PocketBase’s **SQLite** database (by default under `pb_data/`). There is no published Docker image or third-party one-click installer for this fork; deploy by building the binary yourself or wrapping it in your own container.

## Requirements

- Go (see `go.mod` for the toolchain)
- Node.js/npm (to build the admin frontend from source)

## Build and run

1. Clone the repository: `git clone https://github.com/compdani/list_pocket.git`
2. Copy `config.toml.sample` to `config.toml` and adjust `[app]` (e.g. `address`) as needed.
3. Build the frontend and backend (see the [repository README](https://github.com/compdani/list_pocket#build) or [Developer setup](developer-setup.md)).
4. Run the `listpocket` binary and open the app (default `http://127.0.0.1:9000`).

Environment variables override `config.toml` when prefixed with `LISTPOCKET_`; see [Configuration](configuration.md).

## Database and migrations

All application data goes through PocketBase. Schema migrations run when the application starts. The `--install` and `--upgrade` CLI flags exist for compatibility with older listmonk-style automation; in PocketBase mode they only print a message and exit.

## Relationship to listmonk

List Pocket inherits concepts and APIs from [listmonk](https://listmonk.app). Upstream guides that assume a standalone **PostgreSQL** server and `LISTMONK_*` / `[db]` settings describe stock listmonk, not this project’s PocketBase-only stack.
