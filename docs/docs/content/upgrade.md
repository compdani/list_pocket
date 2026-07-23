# Upgrade

## Before upgrading

Always back up **PocketBase’s data** (`pb_data/` by default, or the directory you configured) and any uploaded media or custom static paths before upgrading.

## Standard upgrade

1. Stop the running `listpocket` process.
2. Replace the binary with a newly built version from the desired commit or release tag.
3. Start the application again. Migrations run on startup.

If you run under systemd or another supervisor, use the usual `stop` / `start` cycle around the binary swap.

## CLI flags `--install` and `--upgrade`

These flags are deprecated no-ops kept for compatibility with older listmonk automation. Migrations run on `serve`/startup (or via `listpocket migrate up`). Prefer backing up `pb_data` and restarting with a new binary.

## Coming from upstream listmonk

Stock listmonk (PostgreSQL + standalone app) and List Pocket (PocketBase-only) differ in storage and auth. Migrating an existing listmonk instance is not a simple binary swap; plan a dedicated migration (export/import, API sync, or manual data move) and consult the project repository for any migration notes.
