# Upgrade

## Before upgrading

Always take a **backup of your PostgreSQL database** (and any files your deployment uses for PocketBase data and uploads) before upgrading.

## Standard upgrade

1. Stop the running `listpocket` process.
2. Replace the binary with a newly built version from the desired commit or release tag.
3. Start the application again. Migrations run on startup.

If you run under systemd or another supervisor, use the usual `stop` / `start` cycle around the binary swap.

## CLI flags `--install` and `--upgrade`

These flags are kept for compatibility with older listmonk automation. In List Pocket’s PocketBase mode they do not apply SQL migrations themselves; migrations run when the app starts. Prefer backing up the database and restarting with a new binary.

## Coming from upstream listmonk

Stock listmonk and List Pocket differ in database layout, auth, and packaging. Migrating an existing listmonk instance is not a simple binary swap; plan a dedicated migration (export/import, API sync, or manual data move) and consult the project repository for any migration notes.
