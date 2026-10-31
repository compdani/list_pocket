package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
)

const (
	maxSourceFiles = 100
	maxSourceBytes = 2 << 20
	// MaxReadBytes keeps an installed plugin's source small enough to hand to a model.
	MaxReadBytes = 512 << 10
)

// sourceExts are the files a plugin's source can be read and written as. Anything else,
// such as an image or a bundled binary, can only arrive in an uploaded zip.
var sourceExts = []string{".js", ".mjs", ".json", ".css", ".md", ".txt", ".svg", ".html"}

// SourceFile is one file of a plugin's source, as text.
type SourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CheckedSource is what a set of source files would install.
type CheckedSource struct {
	Manifest *Manifest `json:"manifest"`
	// Warnings are for the person reviewing the source.
	Warnings []string `json:"warnings"`
	// Replaces is the installed version these files would replace, if any.
	Replaces string `json:"replaces"`
}

func checkSourceFile(f SourceFile) error {
	if !isLocalPath(f.Path) || !slices.Contains(sourceExts, path.Ext(f.Path)) {
		return fmt.Errorf("%q must be a relative path ending in one of %s", f.Path, strings.Join(sourceExts, " "))
	}
	if !utf8.ValidString(f.Content) || strings.ContainsRune(f.Content, 0) {
		return fmt.Errorf("%s isn't text", f.Path)
	}
	return nil
}

// CheckSource validates source files without writing them or running any of their code.
// A script only runs once a person has seen it and enabled the plugin.
func (m *Manager) CheckSource(files []SourceFile) (*CheckedSource, error) {
	if len(files) == 0 || len(files) > maxSourceFiles {
		return nil, fmt.Errorf("a plugin needs 1-%d files", maxSourceFiles)
	}
	byPath := make(map[string]string, len(files))
	total := 0
	for _, f := range files {
		if err := checkSourceFile(f); err != nil {
			return nil, err
		}
		if _, dup := byPath[f.Path]; dup {
			return nil, fmt.Errorf("%s appears twice", f.Path)
		}
		byPath[f.Path] = f.Content
		total += len(f.Content)
	}
	if total > maxSourceBytes {
		return nil, fmt.Errorf("the source is over %d bytes", maxSourceBytes)
	}

	manifest, ok := byPath[manifestFile]
	if !ok {
		return nil, fmt.Errorf("%s is missing", manifestFile)
	}
	man, err := ParseManifest([]byte(manifest))
	if err != nil {
		return nil, err
	}
	required := []string{man.Server.Entry}
	if man.UI != nil {
		required = append(append(required, man.UI.Entry), man.UI.CSS...)
	}
	for _, p := range man.I18n {
		required = append(required, p)
		if content, ok := byPath[p]; ok && !json.Valid([]byte(content)) {
			return nil, fmt.Errorf("%s isn't valid JSON", p)
		}
	}
	for _, p := range required {
		if _, ok := byPath[p]; !ok {
			return nil, fmt.Errorf("%s is missing", p)
		}
	}
	if _, err := goja.Compile(man.Server.Entry, byPath[man.Server.Entry], false); err != nil {
		return nil, fmt.Errorf("compile %s: %w", man.Server.Entry, err)
	}

	row, exists, err := m.store.get(man.ID)
	if err != nil {
		return nil, err
	}
	if exists && row.Enabled && row.Version == man.Version {
		return nil, fmt.Errorf("version %s is enabled and running; bump the version to upgrade", man.Version)
	}
	out := &CheckedSource{Manifest: man, Warnings: sourceWarnings(man)}
	if exists {
		out.Replaces = row.Version
		if row.Enabled {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s is enabled. This version goes live the next time the app restarts, with no further approval.", man.Name))
			return out, nil
		}
	}
	out.Warnings = append(out.Warnings, "None of this code runs until you enable the plugin under Settings → Plugins.")
	return out, nil
}

// sourceWarnings points out what lets a plugin act without a person or a signature.
func sourceWarnings(man *Manifest) []string {
	out := []string{}
	for _, w := range man.Webhooks {
		if w.Verify == verifyNone {
			out = append(out, fmt.Sprintf("Webhook %q is public and doesn't check who's calling it.", w.Path))
		}
	}
	for _, t := range man.Tools {
		if !t.ReadOnly && !t.NeedsApproval() {
			out = append(out, fmt.Sprintf("Tool %q can make changes with no approval.", t.Name))
		}
	}
	return out
}

// InstallSource installs source files the same way as an uploaded zip. A new plugin
// stays disabled. Files that can't be written as text, such as images, carry over from the
// installed version. Call CheckSource first to tell a bad proposal from a failed install.
func (m *Manager) InstallSource(files []SourceFile) (*InstallResult, error) {
	checked, err := m.CheckSource(files)
	if err != nil {
		return nil, err
	}
	kept := map[string][]byte{}
	if checked.Replaces != "" {
		if kept, err = m.binaryFiles(checked.Manifest.ID); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.Path)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.Content)); err != nil {
			return nil, err
		}
	}
	for p, content := range kept {
		w, err := zw.Create(p)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return m.Install(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
}

// InstalledSource is an installed plugin's source as text. Settings aren't part of it,
// so it holds no stored secrets.
type InstalledSource struct {
	Version string       `json:"version"`
	Files   []SourceFile `json:"files"`
	// Binary lists files that aren't text source. They carry over to new versions.
	Binary []string `json:"binary,omitempty"`
	// Omitted lists text files that didn't fit. Read them one at a time.
	Omitted []string `json:"omitted,omitempty"`
}

// binaryFiles reads an installed plugin's files that aren't text source.
func (m *Manager) binaryFiles(id string) (map[string][]byte, error) {
	row, ok, err := m.store.get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	src, err := m.ReadSource(id, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(src.Binary))
	root, err := os.OpenRoot(m.versionDir(id, row.Version))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, p := range src.Binary {
		content, err := fs.ReadFile(root.FS(), p)
		if err != nil {
			return nil, err
		}
		out[p] = content
	}
	return out, nil
}

// ReadSource returns an installed plugin's source, up to maxBytes of it. 0 means all of it.
func (m *Manager) ReadSource(id string, maxBytes int) (*InstalledSource, error) {
	row, ok, err := m.store.get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	root, err := os.OpenRoot(m.versionDir(id, row.Version))
	if err != nil {
		return nil, err
	}
	defer root.Close()

	out := &InstalledSource{Version: row.Version, Files: []SourceFile{}}
	total := 0
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !slices.Contains(sourceExts, path.Ext(p)) {
			out.Binary = append(out.Binary, p)
			return nil
		}
		content, err := fs.ReadFile(root.FS(), p)
		if err != nil {
			return err
		}
		if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			out.Binary = append(out.Binary, p)
			return nil
		}
		if maxBytes > 0 && total+len(content) > maxBytes {
			out.Omitted = append(out.Omitted, p)
			return nil
		}
		total += len(content)
		out.Files = append(out.Files, SourceFile{Path: p, Content: string(content)})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return out, err
}
