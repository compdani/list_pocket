package main

import (
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/v2"
	pbcore "github.com/pocketbase/pocketbase/core"
)

// registerDocsRoutes serves the MkDocs build at /docs/, the OpenAPI file at /openapi.yaml,
// and Swagger UI at /swagger/ when the configured or default paths exist on disk.
func registerDocsRoutes(se *pbcore.ServeEvent, ko *koanf.Koanf) {
	swaggerDir := resolveSwaggerDir(ko)
	if swaggerDir != "" {
		openAPI := filepath.Join(swaggerDir, "collections.yaml")
		if fi, err := os.Stat(openAPI); err == nil && !fi.IsDir() {
			se.Router.GET("/openapi.yaml", func(e *pbcore.RequestEvent) error {
				e.Response.Header().Set("Content-Type", "application/yaml; charset=utf-8")
				http.ServeFile(e.Response, e.Request, openAPI)
				return nil
			})
			se.Router.GET("/swagger", func(e *pbcore.RequestEvent) error {
				http.Redirect(e.Response, e.Request, "/swagger/", http.StatusMovedPermanently)
				return nil
			})
			se.Router.GET("/swagger/", func(e *pbcore.RequestEvent) error {
				e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, err := io.WriteString(e.Response, swaggerUIHTML())
				return err
			})
			lo.Printf("API docs: OpenAPI at /openapi.yaml (file: %s); Swagger UI at /swagger/", openAPI)
		}
	}

	docsDir := resolveDocsDir(ko)
	if docsDir == "" {
		return
	}
	if fi, err := os.Stat(docsDir); err != nil || !fi.IsDir() {
		lo.Printf("app.docs_dir %q not found or not a directory; skipping /docs", docsDir)
		return
	}
	lo.Printf("documentation site at /docs/ (MkDocs output: %s)", docsDir)

	se.Router.GET("/docs", func(e *pbcore.RequestEvent) error {
		http.Redirect(e.Response, e.Request, "/docs/", http.StatusMovedPermanently)
		return nil
	})
	se.Router.GET("/docs/", func(e *pbcore.RequestEvent) error {
		http.ServeFile(e.Response, e.Request, filepath.Join(docsDir, "index.html"))
		return nil
	})
	se.Router.GET("/docs/{path...}", func(e *pbcore.RequestEvent) error {
		p := strings.TrimPrefix(strings.TrimSpace(e.Request.PathValue("path")), "/")
		rel := strings.TrimPrefix(path.Clean("/"+p), "/")
		if rel == "" || rel == "." {
			http.ServeFile(e.Response, e.Request, filepath.Join(docsDir, "index.html"))
			return nil
		}
		fullPath := safeJoinUnderDir(docsDir, rel)
		if fullPath == "" {
			http.NotFound(e.Response, e.Request)
			return nil
		}
		fi, err := os.Stat(fullPath)
		if err != nil {
			http.NotFound(e.Response, e.Request)
			return nil
		}
		if fi.IsDir() {
			idx := filepath.Join(fullPath, "index.html")
			if _, err := os.Stat(idx); err == nil {
				http.ServeFile(e.Response, e.Request, idx)
				return nil
			}
			http.NotFound(e.Response, e.Request)
			return nil
		}
		http.ServeFile(e.Response, e.Request, fullPath)
		return nil
	})
}

func safeJoinUnderDir(base, rel string) string {
	base = filepath.Clean(base)
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	candidate := filepath.Join(base, filepath.FromSlash(rel))
	if r, err := filepath.Rel(base, candidate); err != nil || strings.HasPrefix(r, "..") {
		return ""
	}
	return candidate
}

func resolveDocsDir(ko *koanf.Koanf) string {
	d := strings.TrimSpace(ko.String("app.docs_dir"))
	if d != "" {
		return filepath.Clean(d)
	}
	cand := filepath.Join(appDir, "docs/docs/_out")
	if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
		return cand
	}
	return ""
}

func resolveSwaggerDir(ko *koanf.Koanf) string {
	d := strings.TrimSpace(ko.String("app.swagger_dir"))
	if d != "" {
		return filepath.Clean(d)
	}
	cand := filepath.Join(appDir, "docs/swagger")
	if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
		return cand
	}
	return ""
}

func swaggerUIHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>List Pocket API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" crossorigin="anonymous"/>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin="anonymous"></script>
<script>
  window.onload = function() {
    SwaggerUIBundle({
      url: window.location.origin + '/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
    });
  };
</script>
</body>
</html>`
}
