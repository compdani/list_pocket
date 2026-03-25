# listpocket

listpocket is a self-hosted newsletter and mailing list manager built on top of listmonk and adapted around a Go backend, a Vue 3 + Vuetify admin frontend, and PocketBase integration.

Maintained by Danilo Urrutia.

## What It Is

listpocket is intended for running and managing newsletters from your own infrastructure while keeping the application stack straightforward to develop and deploy.

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

With the repo layout unchanged, the server also exposes **`/openapi.yaml`** and **`/swagger/`** (from `docs/swagger/`) and, after `mkdocs build` in `docs/docs`, the manual at **`/docs/`**.

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

## Developers

listpocket is free and open source software licensed under AGPLv3. The backend is written in Go and the frontend is Vue with Vuetify for UI.

## License

listpocket is licensed under the AGPL v3 license.

## Credits

- [listmonk](https://listmonk.app) for the original foundation and newsletter manager architecture.
- [Vuetify](https://vuetifyjs.com) for UI inspiration and design direction.
- [PocketBase](https://pocketbase.io) for the embedded backend, auth, and data platform used in this project.
