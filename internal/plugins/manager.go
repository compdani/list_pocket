// Package plugins installs and runs plugins: a zip with a manifest, server-side
// JavaScript run in goja, and an optional browser module. Plugins are trusted
// like app code, and only super admins can install them.
package plugins

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

const stagingDir = ".staging"

// ErrNotFound means no plugin is installed under that ID.
var ErrNotFound = errors.New("plugin not found")

// HostServices is what plugins need from the rest of the app. It keeps this
// package from depending on cmd.
type HostServices interface {
	LoadPluginSettings(id string) (map[string]any, error)
	SavePluginSettings(id string, values map[string]any) error
	DeletePluginSettings(id string) error
	StripRolePermissions(perms []string) error

	FindContact(channel, address string) (map[string]any, bool, error)
	GetContact(id string) (map[string]any, bool, error)
	UpsertContact(in ContactInput) (map[string]any, error)
	SetConsent(subscriberID, listID, channel, status, source string) error
	ImportSMSConsent(channel string, dryRun, includeInherited bool) (map[string]any, error)
	ListEvents(subscriberID, channel string, limit int) ([]map[string]any, error)
	CountEvents(campaignID string) (map[string]int, error)
	MarkAddressInvalid(subscriberID, channel, address string) error
	RecordEvent(in EventInput) (bool, error)
	UpdateEventStatus(channel, providerMsgID, status string, permanent bool) (bool, error)
	RecordBounce(in BounceInput) error
	OptOut(channel, address string) (int64, error)
	SendMessage(in SendInput) error
}

// Options configures a Manager.
type Options struct {
	Dir      string
	SafeMode bool
	Log      *log.Logger
	Host     HostServices
}

// Manager owns installed plugins and the ones running in this process.
type Manager struct {
	dir      string
	safeMode bool
	log      *log.Logger
	host     HostServices
	store    *store

	mu     sync.RWMutex
	loaded map[string]*Plugin
	sink   func(string, map[string]any)

	draftMu sync.Mutex
}

// New returns a Manager. Call Boot to load enabled plugins.
func New(app core.App, opt Options) *Manager {
	return &Manager{
		dir:      opt.Dir,
		safeMode: opt.SafeMode,
		log:      opt.Log,
		host:     opt.Host,
		store:    &store{app: app},
		loaded:   map[string]*Plugin{},
	}
}

// Info describes an installed plugin for the admin API.
type Info struct {
	Manifest  *Manifest `json:"manifest"`
	Version   string    `json:"version"`
	Enabled   bool      `json:"enabled"`
	Status    string    `json:"status"`
	LastError string    `json:"last_error"`
}

// Boot records plugin directories dropped on disk, which stay disabled, and loads
// the enabled plugins. A plugin that fails to load never stops the app.
func (m *Manager) Boot() error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	if err := m.discover(); err != nil {
		return err
	}
	m.pruneDrafts()
	rows, err := m.store.list()
	if err != nil {
		return err
	}

	for _, row := range rows {
		switch {
		case !row.Enabled:
			err = m.store.setStatus(row.PluginID, StatusDisabled, "")
		case m.safeMode:
			err = m.store.setStatus(row.PluginID, StatusSafeMode, "")
		default:
			if loadErr := m.load(row); loadErr != nil {
				m.log.Printf("plugin %s failed to load: %v", row.PluginID, loadErr)
				err = m.store.setStatus(row.PluginID, StatusFailed, loadErr.Error())
			} else {
				err = m.store.setStatus(row.PluginID, StatusLoaded, "")
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) load(row record) error {
	man, err := m.readManifest(row.PluginID, row.Version)
	if err != nil {
		return err
	}
	p, err := newPlugin(m, man, m.versionDir(row.PluginID, row.Version))
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.loaded[row.PluginID] = p
	m.mu.Unlock()
	return nil
}

// discover adds a disabled row for any plugin directory without one.
func (m *Manager) discover() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || id == stagingDir || !reID.MatchString(id) {
			continue
		}
		if _, ok, err := m.store.get(id); err != nil {
			return err
		} else if ok {
			continue
		}
		version, err := m.newestVersion(id)
		if err != nil {
			m.log.Printf("plugin directory %s skipped: %v", id, err)
			continue
		}
		if err := m.store.upsert(record{PluginID: id, Version: version, Status: StatusDisabled}); err != nil {
			return err
		}
		m.log.Printf("found plugin %s %s on disk; it stays disabled until enabled", id, version)
	}
	return nil
}

func (m *Manager) newestVersion(id string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(m.dir, id))
	if err != nil {
		return "", err
	}
	type candidate struct {
		version string
		modTime int64
	}
	var found []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		man, err := m.readManifest(id, entry.Name())
		if err != nil || man.Version != entry.Name() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		found = append(found, candidate{entry.Name(), info.ModTime().UnixNano()})
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no valid version directory with a %s", manifestFile)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].modTime > found[j].modTime })
	return found[0].version, nil
}

func (m *Manager) versionDir(id, version string) string {
	return filepath.Join(m.dir, id, version)
}

func (m *Manager) readManifest(id, version string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(m.versionDir(id, version), manifestFile))
	if err != nil {
		return nil, err
	}
	man, err := ParseManifest(b)
	if err != nil {
		return nil, err
	}
	if man.ID != id {
		return nil, fmt.Errorf("manifest id %q doesn't match directory %q", man.ID, id)
	}
	return man, nil
}

// Loaded returns a running plugin, or nil.
func (m *Manager) Loaded(id string) *Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded[id]
}

// LoadedPlugins returns every running plugin, sorted by ID.
func (m *Manager) LoadedPlugins() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Plugin, 0, len(m.loaded))
	for _, p := range m.loaded {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out
}

// List returns every installed plugin. One with an unreadable manifest is still
// listed, so it can be seen and uninstalled.
func (m *Manager) List() ([]Info, error) {
	rows, err := m.store.list()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(rows))
	for _, row := range rows {
		info := Info{Version: row.Version, Enabled: row.Enabled, Status: row.Status, LastError: row.LastError}
		man, err := m.readManifest(row.PluginID, row.Version)
		if err != nil {
			man = &Manifest{ID: row.PluginID, Name: row.PluginID, Version: row.Version}
			if info.LastError == "" {
				info.LastError = err.Error()
			}
		}
		info.Manifest = man
		out = append(out, info)
	}
	return out, nil
}

// Manifest returns an installed plugin's manifest, running or not.
func (m *Manager) Manifest(id string) (*Manifest, error) {
	row, ok, err := m.store.get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	return m.readManifest(id, row.Version)
}

// InstalledPermissions returns the permission groups of all installed plugins.
// Disabled ones count too, so roles that hold their permissions stay valid.
func (m *Manager) InstalledPermissions() ([]PermissionGroup, error) {
	rows, err := m.store.list()
	if err != nil {
		return nil, err
	}
	var out []PermissionGroup
	for _, row := range rows {
		man, err := m.readManifest(row.PluginID, row.Version)
		if err != nil {
			continue
		}
		out = append(out, man.Permissions...)
	}
	return out, nil
}

// InstallResult reports an install. NeedsRestart is set when an enabled plugin was
// upgraded, because the running code stays on the old version until a restart.
type InstallResult struct {
	Manifest     *Manifest `json:"manifest"`
	NeedsRestart bool      `json:"needs_restart"`
}

// Install validates a zip and puts it in place. The plugin stays disabled unless
// this is an upgrade of an enabled plugin.
func (m *Manager) Install(src io.ReaderAt, size int64) (*InstallResult, error) {
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}

	staging := filepath.Join(m.dir, stagingDir, security.RandomString(16))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	if err := extractArchive(zr, staging); err != nil {
		return nil, err
	}
	root, err := archiveRoot(staging)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(root, manifestFile))
	if err != nil {
		return nil, err
	}
	man, err := ParseManifest(b)
	if err != nil {
		return nil, err
	}
	if err := checkPackageFiles(root, man); err != nil {
		return nil, err
	}
	// Compiling catches syntax errors here. Running the script waits for enable.
	if _, err := compileEntry(root, man); err != nil {
		return nil, err
	}

	row, exists, err := m.store.get(man.ID)
	if err != nil {
		return nil, err
	}
	if exists && row.Enabled && row.Version == man.Version {
		return nil, fmt.Errorf("version %s is enabled and running; bump the version to upgrade", man.Version)
	}

	dest := m.versionDir(man.ID, man.Version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(dest); err != nil {
		return nil, err
	}
	if err := os.Rename(root, dest); err != nil {
		return nil, err
	}

	// Keep only this version and the one it replaces, which remains on disk for rollback.
	if err := m.pruneVersions(man.ID, man.Version, row.Version); err != nil {
		m.log.Printf("plugin %s: pruning old versions: %v", man.ID, err)
	}

	status := StatusDisabled
	if exists && row.Enabled {
		status = row.Status
	}
	if err := m.store.upsert(record{PluginID: man.ID, Version: man.Version, Enabled: exists && row.Enabled, Status: status}); err != nil {
		return nil, err
	}
	return &InstallResult{Manifest: man, NeedsRestart: exists && row.Enabled}, nil
}

func checkPackageFiles(root string, man *Manifest) error {
	files := []string{man.Server.Entry}
	if man.UI != nil {
		files = append(files, man.UI.Entry)
		files = append(files, man.UI.CSS...)
	}
	for _, p := range man.I18n {
		files = append(files, p)
	}
	for _, f := range files {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("archive is missing %s", f)
		}
	}
	return nil
}

func (m *Manager) pruneVersions(id string, keep ...string) error {
	entries, err := os.ReadDir(filepath.Join(m.dir, id))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == keep[0] || name == keep[1] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.dir, id, name)); err != nil {
			return err
		}
	}
	return nil
}

// SetEnabled flips the flag and returns true if a restart is needed for it to take
// effect. Disabling also stops serving the plugin's routes straight away.
func (m *Manager) SetEnabled(id string, enabled bool) (bool, error) {
	row, ok, err := m.store.get(id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	if row.Enabled == enabled {
		return false, nil
	}
	if enabled {
		if _, err := m.readManifest(id, row.Version); err != nil {
			return false, err
		}
	}
	row.Enabled = enabled
	row.LastError = ""
	row.Status = StatusDisabled
	if enabled {
		// Boot replaces this once the restart loads it.
		row.Status = StatusPending
	}
	if err := m.store.upsert(row); err != nil {
		return false, err
	}
	if !enabled {
		m.mu.Lock()
		delete(m.loaded, id)
		m.mu.Unlock()
	}
	return true, nil
}

// Uninstall removes a disabled plugin's files, and strips its permissions from
// roles because role validation rejects unknown permissions. With purge it also
// deletes the plugin's settings and KV data.
func (m *Manager) Uninstall(id string, purge bool) error {
	row, ok, err := m.store.get(id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if row.Enabled {
		return errors.New("disable the plugin before uninstalling it")
	}

	if man, err := m.readManifest(id, row.Version); err == nil {
		var perms []string
		for _, g := range man.Permissions {
			perms = append(perms, g.Permissions...)
		}
		if len(perms) > 0 {
			if err := m.host.StripRolePermissions(perms); err != nil {
				return fmt.Errorf("remove permissions from roles: %w", err)
			}
		}
	}

	if purge {
		if err := m.host.DeletePluginSettings(id); err != nil {
			return err
		}
		if err := m.store.kvPurge(id); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(m.dir, id)); err != nil {
		return err
	}
	if err := m.DeleteDraft(id); err != nil {
		m.log.Printf("plugin %s: removing its draft: %v", id, err)
	}
	m.mu.Lock()
	delete(m.loaded, id)
	m.mu.Unlock()
	return m.store.delete(id)
}

// SettingsView returns settings safe to send to the browser. Secret values are
// replaced by a has_<key> style entry in the second map.
func (m *Manager) SettingsView(id string) (map[string]any, map[string]bool, error) {
	man, err := m.Manifest(id)
	if err != nil {
		return nil, nil, err
	}
	stored, err := m.host.LoadPluginSettings(id)
	if err != nil {
		return nil, nil, err
	}
	values := map[string]any{}
	secrets := map[string]bool{}
	for _, f := range man.Settings {
		val, ok := stored[f.Key]
		if f.Type == "secret" {
			s, _ := val.(string)
			secrets[f.Key] = s != ""
			continue
		}
		if !ok {
			val = f.Default
		}
		values[f.Key] = val
	}
	return values, secrets, nil
}

// SaveSettings validates and stores settings. An empty secret keeps the current
// value, and keys the manifest doesn't declare are dropped. It takes effect
// straight away because handlers read settings on each call.
func (m *Manager) SaveSettings(id string, update map[string]any) error {
	man, err := m.Manifest(id)
	if err != nil {
		return err
	}
	stored, err := m.host.LoadPluginSettings(id)
	if err != nil {
		return err
	}

	next := map[string]any{}
	for _, f := range man.Settings {
		val, provided := update[f.Key]
		if s, isString := val.(string); f.Type == "secret" && (!provided || (isString && s == "")) {
			val, provided = stored[f.Key]
		} else if !provided {
			val, provided = stored[f.Key]
		}
		if err := validateFieldValue(f, val, provided); err != nil {
			return err
		}
		if provided {
			next[f.Key] = val
		}
	}
	return m.host.SavePluginSettings(id, next)
}

func validateFieldValue(f Field, val any, provided bool) error {
	empty := !provided || val == nil || val == ""
	if empty {
		if f.Required {
			return fmt.Errorf("%s is required", f.Label)
		}
		return nil
	}
	switch f.Type {
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("%s must be true or false", f.Label)
		}
	case "number":
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("%s must be a number", f.Label)
		}
	case "select":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("%s has an invalid value", f.Label)
		}
		for _, opt := range f.Options {
			if opt == s {
				return nil
			}
		}
		return fmt.Errorf("%s has an invalid value", f.Label)
	default:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("%s must be text", f.Label)
		}
	}
	return nil
}

// ReadI18n returns a plugin's message file for a language, if it ships one.
func (p *Plugin) ReadI18n(lang string) ([]byte, bool) {
	rel, ok := p.Manifest.I18n[lang]
	if !ok {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	return b, true
}

// UIManifest is what the admin UI needs to load a running plugin.
type UIManifest struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	Version      string                     `json:"version"`
	Entry        string                     `json:"entry,omitempty"`
	CSS          []string                   `json:"css,omitempty"`
	Settings     []Field                    `json:"settings,omitempty"`
	Messages     map[string]json.RawMessage `json:"messages,omitempty"`
	FieldEditors []string                   `json:"fieldEditors,omitempty"`
}

// UIManifests lists running plugins for the browser. Settings fields are only
// included for super admins, because only they can use the settings API.
func (m *Manager) UIManifests(includeSettings bool) []UIManifest {
	running := m.LoadedPlugins()
	out := make([]UIManifest, 0, len(running))
	for _, p := range running {
		man := p.Manifest
		base := "/admin/plugin-assets/" + man.ID + "/" + url.PathEscape(man.Version) + "/"
		item := UIManifest{ID: man.ID, Name: man.Name, Version: man.Version}
		if man.UI != nil {
			item.Entry = base + man.UI.Entry
			for _, css := range man.UI.CSS {
				item.CSS = append(item.CSS, base+css)
			}
		}
		if includeSettings {
			item.Settings = man.Settings
		}
		for _, f := range man.FieldEditors {
			item.FieldEditors = append(item.FieldEditors, f.Name)
		}
		for lang := range man.I18n {
			raw, ok := p.ReadI18n(lang)
			if !ok || !json.Valid(raw) {
				continue
			}
			if item.Messages == nil {
				item.Messages = map[string]json.RawMessage{}
			}
			item.Messages[lang] = raw
		}
		if item.Entry == "" && len(item.Settings) == 0 && item.Messages == nil {
			continue
		}
		out = append(out, item)
	}
	return out
}
