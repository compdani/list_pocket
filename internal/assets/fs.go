// Package assets builds an in-memory fs.FS from embedded files plus disk overlays.
package assets

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing/fstest"
)

// Opt controls which on-disk directories overlay the embedded files.
type Opt struct {
	FrontendDir string
	StaticDir   string
	I18nDir     string
}

// New copies embed plus optional disk trees into a memory FS.
//
// Embedded layout is remapped so static/public becomes public/. Disk overlays
// (frontend as /admin, --static-dir, --i18n-dir, or cwd static/i18n) replace
// matching virtual paths.
func New(embed fs.FS, opt Opt) (fs.FS, error) {
	m := fstest.MapFS{}
	if err := addFS(m, embed, remapEmbed); err != nil {
		return nil, err
	}

	if dir := existingDir(opt.FrontendDir); dir != "" {
		if err := addDir(m, dir, "admin"); err != nil {
			return nil, err
		}
	}
	if dir := existingDir(opt.I18nDir); dir != "" {
		if err := addDir(m, dir, "i18n"); err != nil {
			return nil, err
		}
	} else if dir := existingDir("i18n"); opt.I18nDir == "" && dir != "" {
		if err := addDir(m, dir, "i18n"); err != nil {
			return nil, err
		}
	}
	if dir := existingDir(opt.StaticDir); dir != "" {
		if err := addStaticDir(m, dir); err != nil {
			return nil, err
		}
	} else if dir := existingDir("static"); opt.StaticDir == "" && dir != "" {
		if err := addStaticDir(m, dir); err != nil {
			return nil, err
		}
	}

	return m, nil
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

func remapEmbed(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "static/public" || strings.HasPrefix(name, "static/public/") {
		return strings.TrimPrefix(name, "static/")
	}
	return name
}

func addFS(m fstest.MapFS, fsys fs.FS, remap func(string) string) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || p == "." {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		virt := p
		if remap != nil {
			virt = remap(p)
		}
		m[virt] = &fstest.MapFile{Data: b}
		return nil
	})
}

func addDir(m fstest.MapFS, root, virtPrefix string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		virt := path.Join(virtPrefix, filepath.ToSlash(rel))
		m[virt] = &fstest.MapFile{Data: b}
		return nil
	})
}

func addStaticDir(m fstest.MapFS, root string) error {
	email := filepath.Join(root, "email-templates")
	if existingDir(email) != "" {
		if err := addDir(m, email, "static/email-templates"); err != nil {
			return err
		}
	}
	pub := filepath.Join(root, "public")
	if existingDir(pub) != "" {
		if err := addDir(m, pub, "public"); err != nil {
			return err
		}
	}
	return nil
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
