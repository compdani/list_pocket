package main

import (
	"html/template"
	stdfs "io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/sprig/v3"
	listpocket "github.com/compdani/list_pocket"
	"github.com/compdani/list_pocket/internal/assets"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
)

// initFS loads embedded static assets and optional on-disk overlays.
func initFS(appDir, frontendDir, staticDir, i18nDir string) stdfs.FS {
	if frontendDir == "" {
		frontendDir = filepath.Join(appDir, "frontend/dist")
	}
	if staticDir != "" {
		lo.Printf("loading static files from: %v", staticDir)
	}
	if i18nDir != "" {
		lo.Printf("loading i18n files from: %v", i18nDir)
	}
	fsys, err := assets.New(listpocket.Files, assets.Opt{
		FrontendDir: frontendDir,
		StaticDir:   staticDir,
		I18nDir:     i18nDir,
	})
	if err != nil {
		lo.Fatalf("failed to initialize asset filesystem: %v", err)
	}
	return fsys
}

func registerServeRoutes(se *pbcore.ServeEvent, app *App) {
	se.Router.BindFunc(func(e *pbcore.RequestEvent) error {
		e.Set("app", app)
		return e.Next()
	})

	registerDocsRoutes(se, ko)

	se.Router.GET("/public/static/{path...}", apis.Static(mustSubFS(app.fs, "/public/static"), false))
	se.Router.GET("/admin/static/{path...}", apis.Static(mustSubFS(app.fs, "/admin/static"), false))

	var (
		uploadProvider = ko.String("upload.provider")
		uploadFsURI    = ko.String("upload.filesystem.upload_uri")
		publicURL      = ko.String("upload.s3.public_url")
	)
	switch {
	case uploadProvider == "filesystem" && uploadFsURI != "":
		staticPath := ko.String("upload.filesystem.upload_path")
		se.Router.GET(path.Join(uploadFsURI, "{filepath...}"), func(e *pbcore.RequestEvent) error {
			http.StripPrefix(uploadFsURI, http.FileServer(http.Dir(staticPath))).ServeHTTP(e.Response, e.Request)
			return nil
		})
	case uploadProvider == "s3" && strings.HasPrefix(publicURL, "/"):
		se.Router.GET(path.Join(publicURL, "{filepath}"), asHandler(app.ServeS3Media))
	}

	registerHandlers(se.Router, app)
}

// parsePublicTemplates parses public HTML templates used by public RequestEvent handlers.
func parsePublicTemplates(i *i18n.I18n, urlCfg *UrlConfig, fsys stdfs.FS) *template.Template {
	tpl, err := parseTemplatesFS(initTplFuncs(i, urlCfg), fsys, "public/templates/*.html")
	if err != nil {
		lo.Fatalf("error parsing public templates: %v", err)
	}
	return tpl
}

func parseTemplatesFS(funcMap template.FuncMap, fsys stdfs.FS, pattern string) (*template.Template, error) {
	return template.New("").Funcs(funcMap).ParseFS(fsys, strings.TrimPrefix(pattern, "/"))
}

func mustSubFS(fsys stdfs.FS, dir string) stdfs.FS {
	sub, err := assets.Sub(fsys, dir)
	if err != nil {
		lo.Printf("asset subdirectory %s not found: %v", dir, err)
		return emptyFS{}
	}
	return sub
}

type emptyFS struct{}

func (emptyFS) Open(name string) (stdfs.File, error) {
	return nil, &stdfs.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
}

// newTplRenderer builds the shared HTML template renderer for public pages.
func newTplRenderer(tpl *template.Template, cfg *Config, urlCfg *UrlConfig, lang *i18n.I18n) *tplRenderer {
	return &tplRenderer{
		templates:           tpl,
		i18n:                lang,
		SiteName:            cfg.SiteName,
		RootURL:             urlCfg.RootURL,
		LogoURL:             urlCfg.LogoURL,
		FaviconURL:          urlCfg.FaviconURL,
		AssetVersion:        cfg.AssetVersion,
		EnablePublicSubPage: cfg.EnablePublicSubPage,
		EnablePublicArchive: cfg.EnablePublicArchive,
		IndividualTracking:  cfg.Privacy.IndividualTracking,
	}
}

// initCaptcha initializes the captcha service.

func initTplFuncs(i *i18n.I18n, u *UrlConfig) template.FuncMap {
	funcs := template.FuncMap{
		"TrackView": func(...any) template.HTML {
			return template.HTML("")
		},
		"UnsubscribeURL": func(...any) string {
			return ""
		},
		"ManageURL": func(...any) string {
			return ""
		},
		"OptinURL": func(...any) string {
			return ""
		},
		"MessageURL": func(...any) string {
			return ""
		},
		"ArchiveURL": func(...any) string {
			return ""
		},
		"RootURL": func() string {
			return u.RootURL
		},
		"LogoURL": func() string {
			return u.LogoURL
		},
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"L": func() *i18n.I18n {
			return i
		},
		"Safe": func(safeHTML string) template.HTML {
			return template.HTML(safeHTML)
		},
	}

	// Copy spring functions.
	sprigFuncs := sprig.GenericFuncMap()
	delete(sprigFuncs, "env")
	delete(sprigFuncs, "expandenv")
	delete(sprigFuncs, "getHostByName")

	maps.Copy(funcs, sprigFuncs)

	return funcs
}

// initAuth initializes the auth module.
