# Introduction

[![List Pocket](images/logo.svg)](https://github.com/compdani/list_pocket)

**List Pocket** is a self-hosted newsletter and mailing list manager. It keeps the core [listmonk](https://listmonk.app)-style workflow (campaigns, lists, subscribers, APIs) while running entirely on **embedded [PocketBase](https://pocketbase.io)** — auth, collections, realtime, and the app’s SQL layer (SQLite under the hood). There is no separate PostgreSQL or other external database server.

The admin UI is a **Vue 3** app; the server is a single **Go** binary.

## Developers

List Pocket is free and open source software licensed under AGPLv3. See the [GitHub repository](https://github.com/compdani/list_pocket) and the [developer setup](developer-setup.md). The backend is Go; the frontend is Vue with Vuetify.
