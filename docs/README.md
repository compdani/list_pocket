# Static website and docs

This directory holds the static marketing site and MkDocs documentation for **List Pocket**.

- The marketing site lives in `site` and is built with [Hugo](https://gohugo.io/). From `site`, run `hugo serve` to preview locally.

- Documentation lives in `docs/docs` and is built with [MkDocs](https://www.mkdocs.org/). From `docs/docs`, run `pip install -r requirements.txt` (once) and `mkdocs serve` to preview.

- The `i18n` subdirectory contains a small static UI for editing translation JSON files (served alongside docs when deployed).

When the main List Pocket binary runs from the repository root, it serves the built MkDocs site at **`/docs/`** (after `mkdocs build` in `docs/docs`) and the OpenAPI spec plus Swagger UI at **`/openapi.yaml`** and **`/swagger/`**. Override paths with `app.docs_dir` and `app.swagger_dir` in `config.toml` if needed.
