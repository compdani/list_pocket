# Installation

List Pocket ships as source in this repository. It is a **Go** application that uses **PostgreSQL** for relational data and **embedded PocketBase** for auth and related features. There is no published Docker image or third-party one-click installer for this fork; deploy by building the binary yourself or wrapping it in your own container.

## Requirements

- PostgreSQL (create a database and user; configure `[db]` in `config.toml`)
- Go and Node.js/npm (to build from source)

## Build and run

1. Clone the repository: `git clone https://github.com/compdani/list_pocket.git`
2. Copy `config.toml.sample` to `config.toml` and set `[db]` for your Postgres instance.
3. Build the frontend and backend (see the [repository README](https://github.com/compdani/list_pocket#build) or [Developer setup](developer-setup.md)).
4. Run the `listpocket` binary and open the app (default `http://127.0.0.1:9000`).

Environment variables override `config.toml` when prefixed with `LISTPOCKET_`; see [Configuration](configuration.md).

## Database setup

Schema migrations run automatically when the application starts. The `--install` and `--upgrade` CLI flags exist for compatibility with upstream listmonk scripts; in PocketBase mode they only print a message and exit — you do not run a separate SQL install step.

## Relationship to listmonk

List Pocket inherits concepts and APIs from [listmonk](https://listmonk.app). Upstream guides that reference the `listmonk` binary, `LISTMONK_*` variables, or the official Docker image apply to stock listmonk, not this fork.
