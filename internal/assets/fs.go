// Package assets builds an overlay fs.FS from embedded files plus disk overlays.
package assets

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

// Opt controls which on-disk directories overlay the embedded files.
type Opt struct {
	FrontendDir string
	StaticDir   string
	I18nDir     string
}

// New returns a first-hit-wins overlay of disk directories over embedded files.
//
// Embedded layout is remapped so static/public becomes public/. Disk overlays
// (frontend as /admin, --static-dir, --i18n-dir, or cwd static/i18n) replace
// matching virtual paths. The admin UI is served from disk and is never copied
// into memory.
func New(embed fs.FS, opt Opt) (fs.FS, error) {
	layers := make([]fs.FS, 0, 6)

	if dir := existingDir(opt.StaticDir); dir != "" {
		layers = append(layers, staticDirFS(dir)...)
	} else if dir := existingDir("static"); opt.StaticDir == "" && dir != "" {
		layers = append(layers, staticDirFS(dir)...)
	}

	if dir := existingDir(opt.I18nDir); dir != "" {
		layers = append(layers, prefixFS{prefix: "i18n", base: os.DirFS(dir)})
	} else if dir := existingDir("i18n"); opt.I18nDir == "" && dir != "" {
		layers = append(layers, prefixFS{prefix: "i18n", base: os.DirFS(dir)})
	}

	if dir := existingDir(opt.FrontendDir); dir != "" {
		layers = append(layers, prefixFS{prefix: "admin", base: os.DirFS(dir)})
	}

	layers = append(layers, remapFS{base: embed})
	return overlayFS{layers: layers}, nil
}

// ReadFile reads name from fsys, accepting a leading slash.
func ReadFile(fsys fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(fsys, strings.TrimPrefix(name, "/"))
}

// Glob is fs.Glob with a leading slash stripped from the pattern.
func Glob(fsys fs.FS, pattern string) ([]string, error) {
	return fs.Glob(fsys, strings.TrimPrefix(pattern, "/"))
}

// Sub is fs.Sub with a leading slash stripped from dir.
func Sub(fsys fs.FS, dir string) (fs.FS, error) {
	dir = strings.TrimPrefix(dir, "/")
	if dir == "" || dir == "." {
		return fsys, nil
	}
	return fs.Sub(fsys, dir)
}

type overlayFS struct {
	layers []fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}
	var firstErr error
	for _, layer := range o.layers {
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fs.ErrNotExist
	}
	return nil, firstErr
}

func (o overlayFS) Glob(pattern string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, layer := range o.layers {
		matches, err := fs.Glob(layer, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			out = append(out, m)
		}
	}
	return out, nil
}

type remapFS struct {
	base fs.FS
}

func (r remapFS) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}
	if name == "public" || strings.HasPrefix(name, "public/") {
		if f, err := r.base.Open(path.Join("static", name)); err == nil {
			return f, nil
		}
	}
	return r.base.Open(name)
}

type prefixFS struct {
	prefix string
	base   fs.FS
}

func (p prefixFS) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}
	if p.prefix == "" {
		return p.base.Open(name)
	}
	if name == p.prefix {
		return p.base.Open(".")
	}
	pref := p.prefix + "/"
	if strings.HasPrefix(name, pref) {
		return p.base.Open(strings.TrimPrefix(name, pref))
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func staticDirFS(root string) []fs.FS {
	var out []fs.FS
	if dir := existingDir(path.Join(root, "email-templates")); dir != "" {
		out = append(out, prefixFS{prefix: "static/email-templates", base: os.DirFS(dir)})
	}
	if dir := existingDir(path.Join(root, "public")); dir != "" {
		out = append(out, prefixFS{prefix: "public", base: os.DirFS(dir)})
	}
	return out
}

func existingDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return ""
	}
	return dir
}
