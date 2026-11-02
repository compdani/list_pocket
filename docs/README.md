# Static website and docs

This directory holds the static marketing site and Zensical documentation for **List Pocket**.

- The marketing site lives in `site` and is built with [Hugo](https://gohugo.io/). From `site`, run `hugo serve` to preview locally.

- Documentation lives in `docs/docs` and is built with [Zensical](https://zensical.org/). From `docs/docs`, run `pip install -r requirements.txt` (once) and `zensical serve` to preview. The project config stays in `mkdocs.yml`, which Zensical reads directly.

- The `i18n` subdirectory contains a small static UI for editing translation JSON files (served alongside docs when deployed).

When the main List Pocket binary runs from the repository root, it serves the built Zensical site at **`/docs/`** (after `zensical build` in `docs/docs`) and the OpenAPI spec plus Swagger UI at **`/openapi.yaml`** and **`/swagger/`**. Override paths with `app.docs_dir` and `app.swagger_dir` in `config.toml` if needed.
