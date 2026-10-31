package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	draftsDir     = ".drafts"
	draftMaxAge   = 30 * 24 * time.Hour
	maxFileRead   = 256 << 10
	draftFileMode = 0o600
)

// Draft is a plugin's source being written over several steps, so it doesn't have to fit
// in one reply. It's only data: it's never run or served, and a person still reviews the
// finished files. It survives restarts, because enabling a plugin restarts the app.
type Draft struct {
	Plugin string `json:"plugin"`
	// BaseVersion is the installed version it started from, if any.
	BaseVersion string       `json:"base_version"`
	Files       []SourceFile `json:"files"`
}

// DraftEdit replaces one exact, unique piece of a file's text.
type DraftEdit struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// FileInfo describes a file without its content.
type FileInfo struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
	Lines int    `json:"lines"`
}

// DraftInfo is where a draft stands.
type DraftInfo struct {
	Plugin      string     `json:"plugin"`
	BaseVersion string     `json:"base_version,omitempty"`
	Files       []FileInfo `json:"files"`
}

func (d *Draft) info() DraftInfo {
	files := make([]FileInfo, 0, len(d.Files))
	for _, f := range d.Files {
		files = append(files, fileInfo(f))
	}
	return DraftInfo{Plugin: d.Plugin, BaseVersion: d.BaseVersion, Files: files}
}

func fileInfo(f SourceFile) FileInfo {
	return FileInfo{Path: f.Path, Bytes: len(f.Content), Lines: strings.Count(f.Content, "\n") + 1}
}

func (d *Draft) file(path string) *SourceFile {
	for i := range d.Files {
		if d.Files[i].Path == path {
			return &d.Files[i]
		}
	}
	return nil
}

func (d *Draft) checkSize() error {
	total := 0
	for _, f := range d.Files {
		total += len(f.Content)
	}
	if len(d.Files) > maxSourceFiles {
		return fmt.Errorf("a plugin can have at most %d files", maxSourceFiles)
	}
	if total > maxSourceBytes {
		return fmt.Errorf("the source is over %d bytes", maxSourceBytes)
	}
	return nil
}

func validDraftID(id string) error {
	if !reID.MatchString(id) || slices.Contains(reservedIDs, id) {
		return fmt.Errorf("invalid plugin id %q", id)
	}
	return nil
}

func (m *Manager) draftPath(id string) string {
	return filepath.Join(m.dir, draftsDir, id+".json")
}

func (m *Manager) loadDraft(id string) (*Draft, bool, error) {
	raw, err := os.ReadFile(m.draftPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var d Draft
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, false, fmt.Errorf("the draft of %s can't be read: %w", id, err)
	}
	return &d, true, nil
}

func (m *Manager) saveDraft(d *Draft) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	dir := filepath.Join(m.dir, draftsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, d.Plugin+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), draftFileMode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), m.draftPath(d.Plugin))
}

// openDraft returns the plugin's draft, or starts one from what's installed.
func (m *Manager) openDraft(id string, startOver bool) (*Draft, error) {
	if err := validDraftID(id); err != nil {
		return nil, err
	}
	if !startOver {
		if d, ok, err := m.loadDraft(id); err != nil || ok {
			return d, err
		}
	}
	d := &Draft{Plugin: id, Files: []SourceFile{}}
	src, err := m.ReadSource(id, 0)
	if errors.Is(err, ErrNotFound) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	d.BaseVersion, d.Files = src.Version, src.Files
	return d, nil
}

// WriteDraft adds or replaces whole files and removes others. The manifest isn't checked
// until the draft is proposed, because it may not be finished yet.
func (m *Manager) WriteDraft(id string, files []SourceFile, remove []string, startOver bool) (DraftInfo, error) {
	m.draftMu.Lock()
	defer m.draftMu.Unlock()
	d, err := m.openDraft(id, startOver)
	if err != nil {
		return DraftInfo{}, err
	}
	for _, f := range files {
		if err := checkSourceFile(f); err != nil {
			return DraftInfo{}, err
		}
		if existing := d.file(f.Path); existing != nil {
			existing.Content = f.Content
		} else {
			d.Files = append(d.Files, f)
		}
	}
	for _, p := range remove {
		if d.file(p) == nil {
			return DraftInfo{}, fmt.Errorf("%s isn't in the draft", p)
		}
		d.Files = slices.DeleteFunc(d.Files, func(f SourceFile) bool { return f.Path == p })
	}
	if err := d.checkSize(); err != nil {
		return DraftInfo{}, err
	}
	return d.info(), m.saveDraft(d)
}

// EditDraft applies targeted edits to one file, all of them or none. An edit whose text
// isn't there exactly once fails, so it can't change the wrong place.
func (m *Manager) EditDraft(id, path string, edits []DraftEdit) (FileInfo, error) {
	m.draftMu.Lock()
	defer m.draftMu.Unlock()
	if len(edits) == 0 {
		return FileInfo{}, errors.New("edits must not be empty")
	}
	d, err := m.openDraft(id, false)
	if err != nil {
		return FileInfo{}, err
	}
	file := d.file(path)
	if file == nil {
		return FileInfo{}, fmt.Errorf("%s isn't in the draft; add it with plugin_draft_write", path)
	}
	content := file.Content
	for i, edit := range edits {
		if edit.Old == "" {
			return FileInfo{}, fmt.Errorf("edit %d: old must not be empty", i+1)
		}
		switch n := strings.Count(content, edit.Old); n {
		case 1:
			content = strings.Replace(content, edit.Old, edit.New, 1)
		case 0:
			return FileInfo{}, fmt.Errorf("edit %d: old text isn't in %s; read the file and copy it exactly", i+1, path)
		default:
			return FileInfo{}, fmt.Errorf("edit %d: old text appears %d times in %s; include more of the lines around it", i+1, n, path)
		}
	}
	file.Content = content
	if err := checkSourceFile(*file); err != nil {
		return FileInfo{}, err
	}
	if err := d.checkSize(); err != nil {
		return FileInfo{}, err
	}
	return fileInfo(*file), m.saveDraft(d)
}

// PluginFileListing says which files can be read, and whether they're from a draft.
type PluginFileListing struct {
	Source string `json:"source"`
	DraftInfo
}

// PluginFileRead is part of one file.
type PluginFileRead struct {
	Source     string `json:"source"`
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Content    string `json:"content"`
	// Truncated means there's more than one read returns. Continue from EndLine + 1.
	Truncated bool `json:"truncated,omitempty"`
}

// readableSource is the draft if there is one, otherwise what's installed.
func (m *Manager) readableSource(id string) (string, *Draft, error) {
	if err := validDraftID(id); err != nil {
		return "", nil, err
	}
	m.draftMu.Lock()
	defer m.draftMu.Unlock()
	if d, ok, err := m.loadDraft(id); err != nil || ok {
		return "draft", d, err
	}
	d, err := m.openDraft(id, false)
	if err != nil {
		return "", nil, err
	}
	if d.BaseVersion == "" {
		return "none", d, nil
	}
	return "installed", d, nil
}

// ListPluginFiles lists a plugin's draft files, or its installed ones.
func (m *Manager) ListPluginFiles(id string) (PluginFileListing, error) {
	source, d, err := m.readableSource(id)
	if err != nil {
		return PluginFileListing{}, err
	}
	return PluginFileListing{Source: source, DraftInfo: d.info()}, nil
}

// ReadPluginFile reads lines of a file, from the draft if there is one. Lines count from
// 1, and an end of 0 means the end of the file.
func (m *Manager) ReadPluginFile(id, path string, start, end int) (PluginFileRead, error) {
	source, d, err := m.readableSource(id)
	if err != nil {
		return PluginFileRead{}, err
	}
	file := d.file(path)
	if file == nil {
		return PluginFileRead{}, fmt.Errorf("%s isn't a text file of %s", path, id)
	}
	lines := strings.SplitAfter(file.Content, "\n")
	if start < 1 {
		start = 1
	}
	if end < 1 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return PluginFileRead{}, fmt.Errorf("%s has %d lines", path, len(lines))
	}
	out := PluginFileRead{Source: source, Path: path, StartLine: start, TotalLines: len(lines)}
	var content strings.Builder
	last := start - 1
	for i := start - 1; i < end; i++ {
		if content.Len() > 0 && content.Len()+len(lines[i]) > maxFileRead {
			out.Truncated = true
			break
		}
		content.WriteString(lines[i])
		last = i + 1
	}
	out.EndLine, out.Content = last, content.String()
	return out, nil
}

// DraftFiles returns a finished draft's files. It's refused if another version was
// installed after the draft began, because proposing it would undo that version.
func (m *Manager) DraftFiles(id string) ([]SourceFile, error) {
	if err := validDraftID(id); err != nil {
		return nil, err
	}
	m.draftMu.Lock()
	defer m.draftMu.Unlock()
	d, ok, err := m.loadDraft(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("there's no draft of %s; start one with plugin_draft_write or plugin_draft_edit", id)
	}
	row, exists, err := m.store.get(id)
	if err != nil {
		return nil, err
	}
	if exists && row.Version != d.BaseVersion {
		return nil, fmt.Errorf("version %s was installed after this draft began; call plugin_draft_write with start_over and redo the changes", row.Version)
	}
	return d.Files, nil
}

// DeleteDraft removes a plugin's draft, if it has one.
func (m *Manager) DeleteDraft(id string) error {
	if err := validDraftID(id); err != nil {
		return err
	}
	m.draftMu.Lock()
	defer m.draftMu.Unlock()
	if err := os.Remove(m.draftPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// pruneDrafts removes drafts nobody has touched in a long time.
func (m *Manager) pruneDrafts() {
	dir := filepath.Join(m.dir, draftsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < draftMaxAge {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			m.log.Printf("removing old plugin draft %s: %v", entry.Name(), err)
		}
	}
}
