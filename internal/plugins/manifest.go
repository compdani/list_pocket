package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/compdani/list_pocket/models"
)

const (
	manifestFile = "plugin.json"

	defaultConcurrency  = 4
	maxConcurrency      = 16
	defaultTimeoutMs    = 15000
	maxTimeoutMs        = 120000
	defaultWebhookBytes = 1 << 20
	maxWebhookBytes     = 10 << 20

	verifyNone = "none"
)

var (
	reID      = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	reVersion = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]{0,31}$`)
	rePath    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(/[a-z0-9][a-z0-9_-]*)*$`)
	reKey     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

	// reservedIDs would let a plugin claim a built-in channel's ch:<id> messenger.
	reservedIDs = []string{"whatsapp"}

	validMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	fieldTypes   = []string{"text", "textarea", "secret", "select", "number", "boolean"}
)

// Manifest is the static contract in plugin.json.
type Manifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Server        ServerSpec        `json:"server"`
	UI            *UISpec           `json:"ui,omitempty"`
	Channels      []ChannelManifest `json:"channels,omitempty"`
	Webhooks      []WebhookManifest `json:"webhooks,omitempty"`
	AdminRoutes   []AdminRoute      `json:"adminRoutes,omitempty"`
	Settings      []Field           `json:"settings,omitempty"`
	Permissions   []PermissionGroup `json:"permissions,omitempty"`
	I18n          map[string]string `json:"i18n,omitempty"`

	Tools        []ToolManifest        `json:"tools,omitempty"`
	Events       []EventManifest       `json:"events,omitempty"`
	AgentPresets []PresetManifest      `json:"agentPresets,omitempty"`
	FieldEditors []FieldEditorManifest `json:"fieldEditors,omitempty"`
}

// ServerSpec configures the plugin's JavaScript runtime.
type ServerSpec struct {
	Entry       string `json:"entry"`
	Concurrency int    `json:"concurrency"`
	TimeoutMs   int    `json:"timeoutMs"`
}

// UISpec points at the browser module, relative to the plugin root.
type UISpec struct {
	Entry string   `json:"entry"`
	CSS   []string `json:"css,omitempty"`
}

// RateLimit caps how fast a channel sends.
type RateLimit struct {
	MaxPerSecond int `json:"maxPerSecond"`
}

// ChannelManifest declares a sendable channel.
type ChannelManifest struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	Icon           string     `json:"icon,omitempty"`
	AddressSource  string     `json:"addressSource,omitempty"`
	AddressLabel   string     `json:"addressLabel,omitempty"`
	ContentTypes   []string   `json:"contentTypes,omitempty"`
	CampaignBody   string     `json:"campaignBody,omitempty"`
	CampaignFields []Field    `json:"campaignFields,omitempty"`
	RateLimit      *RateLimit `json:"rateLimit,omitempty"`
}

// WebhookManifest declares a public receiver at /webhooks/plugins/<id>/<path>.
type WebhookManifest struct {
	Path         string   `json:"path"`
	Methods      []string `json:"methods,omitempty"`
	MaxBodyBytes int64    `json:"maxBodyBytes,omitempty"`
	Verify       string   `json:"verify,omitempty"`
}

// AdminRoute declares an authenticated route at /mailapi/plugins/<id>/api/<path>.
type AdminRoute struct {
	Path       string   `json:"path"`
	Methods    []string `json:"methods,omitempty"`
	Permission string   `json:"permission"`
}

// Field describes one settings or campaign form field.
type Field struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
	Default  any      `json:"default,omitempty"`
}

// PermissionGroup is merged into the app's permission catalog.
type PermissionGroup struct {
	Group       string   `json:"group"`
	Permissions []string `json:"permissions"`
}

// ParseManifest decodes plugin.json, applies defaults and validates it.
func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", manifestFile, err)
	}
	if err := m.normalize(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", manifestFile, err)
	}
	return &m, nil
}

func (m *Manifest) normalize() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schemaVersion %d", m.SchemaVersion)
	}
	if !reID.MatchString(m.ID) {
		return fmt.Errorf("invalid id %q", m.ID)
	}
	if slices.Contains(reservedIDs, m.ID) {
		return fmt.Errorf("id %q is reserved for a built-in channel", m.ID)
	}
	if !reVersion.MatchString(m.Version) || strings.Contains(m.Version, "..") {
		return fmt.Errorf("invalid version %q", m.Version)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name is required")
	}

	if m.Server.Entry == "" {
		m.Server.Entry = "server.js"
	}
	if !isLocalPath(m.Server.Entry) {
		return fmt.Errorf("invalid server.entry %q", m.Server.Entry)
	}
	if m.Server.Concurrency == 0 {
		m.Server.Concurrency = defaultConcurrency
	}
	if m.Server.Concurrency < 1 || m.Server.Concurrency > maxConcurrency {
		return fmt.Errorf("server.concurrency must be 1-%d", maxConcurrency)
	}
	if m.Server.TimeoutMs == 0 {
		m.Server.TimeoutMs = defaultTimeoutMs
	}
	if m.Server.TimeoutMs < 100 || m.Server.TimeoutMs > maxTimeoutMs {
		return fmt.Errorf("server.timeoutMs must be 100-%d", maxTimeoutMs)
	}

	if m.UI != nil {
		for _, p := range append([]string{m.UI.Entry}, m.UI.CSS...) {
			if !isLocalPath(p) || !strings.HasPrefix(p, "ui/") {
				return fmt.Errorf("ui file %q must be under ui/", p)
			}
		}
	}

	perms, err := m.normalizePermissions()
	if err != nil {
		return err
	}
	if err := m.normalizeChannels(); err != nil {
		return err
	}
	if err := m.normalizeRoutes(perms); err != nil {
		return err
	}
	if err := m.normalizeAddons(); err != nil {
		return err
	}
	if err := validateFields("settings", m.Settings); err != nil {
		return err
	}
	for lang, p := range m.I18n {
		if !isLocalPath(p) {
			return fmt.Errorf("invalid i18n path %q for %q", p, lang)
		}
	}
	return nil
}

func (m *Manifest) normalizePermissions() (map[string]struct{}, error) {
	group := "plugin_" + m.ID
	perms := map[string]struct{}{}
	for _, g := range m.Permissions {
		if g.Group != group {
			return nil, fmt.Errorf("permission group must be %q", group)
		}
		for _, p := range g.Permissions {
			if !strings.HasPrefix(p, group+":") {
				return nil, fmt.Errorf("permission %q must start with %q", p, group+":")
			}
			perms[p] = struct{}{}
		}
	}
	return perms, nil
}

func (m *Manifest) normalizeChannels() error {
	seen := map[string]bool{}
	want := models.ChannelMessengerPrefix + m.ID
	for i := range m.Channels {
		c := &m.Channels[i]
		if (c.ID != want && !strings.HasPrefix(c.ID, want+"_")) || !reKey.MatchString(strings.TrimPrefix(c.ID, models.ChannelMessengerPrefix)) {
			return fmt.Errorf("channel id %q must be %q or start with %q", c.ID, want, want+"_")
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate channel %q", c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Label) == "" {
			return fmt.Errorf("channel %q needs a label", c.ID)
		}
		if c.AddressSource == "" {
			c.AddressSource = string(models.AddressSourceChannel)
		}
		if c.AddressSource != string(models.AddressSourceChannel) && c.AddressSource != string(models.AddressSourcePhone) {
			return fmt.Errorf("channel %q has invalid addressSource %q", c.ID, c.AddressSource)
		}
		if len(c.ContentTypes) == 0 {
			c.ContentTypes = []string{models.CampaignContentTypePlain}
		}
		if len(c.ContentTypes) != 1 || c.ContentTypes[0] != models.CampaignContentTypePlain {
			return fmt.Errorf("channel %q only supports the plain content type", c.ID)
		}
		if c.CampaignBody != "" && c.CampaignBody != models.CampaignBodyNone {
			return fmt.Errorf("channel %q has invalid campaignBody %q", c.ID, c.CampaignBody)
		}
		if c.RateLimit != nil && c.RateLimit.MaxPerSecond < 0 {
			return fmt.Errorf("channel %q has a negative rate limit", c.ID)
		}
		if err := validateFields("campaignFields", c.CampaignFields); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) normalizeRoutes(perms map[string]struct{}) error {
	seen := map[string]bool{}
	for i := range m.Webhooks {
		w := &m.Webhooks[i]
		if !rePath.MatchString(w.Path) || seen[w.Path] {
			return fmt.Errorf("invalid or duplicate webhook path %q", w.Path)
		}
		seen[w.Path] = true
		methods, err := normalizeMethods(w.Methods, "POST")
		if err != nil {
			return err
		}
		w.Methods = methods
		if w.MaxBodyBytes == 0 {
			w.MaxBodyBytes = defaultWebhookBytes
		}
		if w.MaxBodyBytes < 0 || w.MaxBodyBytes > maxWebhookBytes {
			return fmt.Errorf("webhook %q maxBodyBytes must be at most %d", w.Path, maxWebhookBytes)
		}
		if w.Verify != "" && w.Verify != verifyNone {
			return fmt.Errorf("webhook %q has invalid verify %q", w.Path, w.Verify)
		}
	}

	seen = map[string]bool{}
	for i := range m.AdminRoutes {
		r := &m.AdminRoutes[i]
		if !rePath.MatchString(r.Path) || seen[r.Path] {
			return fmt.Errorf("invalid or duplicate admin route %q", r.Path)
		}
		seen[r.Path] = true
		methods, err := normalizeMethods(r.Methods, "GET")
		if err != nil {
			return err
		}
		r.Methods = methods
		if _, ok := perms[r.Permission]; !ok {
			return fmt.Errorf("admin route %q needs a permission declared by this plugin", r.Path)
		}
	}
	return nil
}

func normalizeMethods(in []string, def string) ([]string, error) {
	if len(in) == 0 {
		return []string{def}, nil
	}
	out := make([]string, 0, len(in))
	for _, method := range in {
		method = strings.ToUpper(method)
		if !slices.Contains(validMethods, method) {
			return nil, fmt.Errorf("invalid method %q", method)
		}
		out = append(out, method)
	}
	return out, nil
}

func validateFields(where string, fields []Field) error {
	seen := map[string]bool{}
	for _, f := range fields {
		if !reKey.MatchString(f.Key) || seen[f.Key] {
			return fmt.Errorf("%s: invalid or duplicate key %q", where, f.Key)
		}
		seen[f.Key] = true
		if !slices.Contains(fieldTypes, f.Type) {
			return fmt.Errorf("%s: field %q has invalid type %q", where, f.Key, f.Type)
		}
		if f.Type == "select" && len(f.Options) == 0 {
			return fmt.Errorf("%s: select field %q needs options", where, f.Key)
		}
	}
	return nil
}

func isLocalPath(p string) bool {
	return p != "" && !strings.Contains(p, `\`) && filepath.IsLocal(filepath.FromSlash(p))
}
