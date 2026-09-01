// Package config is a small nested key/value store used to merge TOML files,
// environment variables, CLI flags, and database settings.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
)

const delim = "."

// Conf is a nested map with dotted-key access, similar to the subset of koanf
// this app previously used.
type Conf struct {
	data map[string]any
}

// New returns an empty config.
func New() *Conf {
	return &Conf{data: map[string]any{}}
}

// LoadTOMLFile merges a TOML file into the config. Later loads override earlier ones.
func (c *Conf) LoadTOMLFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := toml.Unmarshal(b, &m); err != nil {
		return err
	}
	c.merge(normalizeMap(m))
	return nil
}

// LoadMap unflattens dotted keys (as stored in listpocket_settings JSON) and merges.
func (c *Conf) LoadMap(m map[string]any) error {
	c.merge(unflatten(normalizeMap(m), delim))
	return nil
}

// LoadEnv merges environment variables with the given prefix.
// LISTPOCKET_foo__bar -> foo.bar; LISTPOCKET_static_dir -> static-dir.
func (c *Conf) LoadEnv(prefix string) error {
	mp := map[string]any{}
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !strings.HasPrefix(k, prefix) {
			continue
		}
		mp[EnvKey(k, prefix)] = v
	}
	c.merge(unflatten(mp, delim))
	return nil
}

// EnvKey transforms an environment variable name into a dotted config key.
func EnvKey(s, prefix string) string {
	key := strings.ToLower(strings.TrimPrefix(s, prefix))
	key = strings.ReplaceAll(key, "__", ".")
	if !strings.Contains(key, ".") {
		key = strings.ReplaceAll(key, "_", "-")
	}
	return key
}

// LoadFlags merges parsed pflag values. Flag names are used as keys as-is.
func (c *Conf) LoadFlags(fs *pflag.FlagSet) error {
	mp := map[string]any{}
	fs.VisitAll(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "bool":
			b, _ := fs.GetBool(f.Name)
			mp[f.Name] = b
		case "stringSlice":
			ss, _ := fs.GetStringSlice(f.Name)
			mp[f.Name] = ss
		case "int":
			i, _ := fs.GetInt(f.Name)
			mp[f.Name] = i
		default:
			mp[f.Name] = f.Value.String()
		}
	})
	c.merge(unflatten(mp, delim))
	return nil
}

// Set writes a dotted key.
func (c *Conf) Set(path string, val any) error {
	if path == "" {
		return fmt.Errorf("empty config key")
	}
	parts := strings.Split(path, delim)
	m := c.data
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = val
	return nil
}

// Exists reports whether the dotted key is present.
func (c *Conf) Exists(path string) bool {
	_, ok := c.lookup(path)
	return ok
}

// Get returns the raw value at path, or nil.
func (c *Conf) Get(path string) any {
	v, _ := c.lookup(path)
	return v
}

// String returns the value as a string.
func (c *Conf) String(path string) string {
	return toString(c.Get(path))
}

// Strings returns a string slice.
func (c *Conf) Strings(path string) []string {
	return toStrings(c.Get(path))
}

// Bool returns the value as a bool.
func (c *Conf) Bool(path string) bool {
	return toBool(c.Get(path))
}

// Int returns the value as an int.
func (c *Conf) Int(path string) int {
	return toInt(c.Get(path))
}

// Duration returns the value as a time.Duration.
func (c *Conf) Duration(path string) time.Duration {
	return toDuration(c.Get(path))
}

// MustString returns a non-empty string or panics.
func (c *Conf) MustString(path string) string {
	s := c.String(path)
	if s == "" {
		panic("config: missing " + path)
	}
	return s
}

// Unmarshal decodes the subtree at path into dest using `config` struct tags.
func (c *Conf) Unmarshal(path string, dest any) error {
	return c.unmarshal(path, dest, "config", false)
}

// UnmarshalJSONTag decodes using `json` struct tags (SMTP, messengers, mailboxes).
func (c *Conf) UnmarshalJSONTag(path string, dest any) error {
	return c.unmarshal(path, dest, "json", false)
}

// UnmarshalFlat flattens nested keys before decoding (`admin.custom_css` tags).
func (c *Conf) UnmarshalFlat(path string, dest any) error {
	return c.unmarshal(path, dest, "config", true)
}

// Slices returns each map in a list as its own Conf.
func (c *Conf) Slices(path string) []*Conf {
	v := c.Get(path)
	sl, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]*Conf, 0, len(sl))
	for _, item := range sl {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, &Conf{data: m})
	}
	return out
}

// LookupStrings turns a string slice into a set.
func LookupStrings(s []string) map[string]bool {
	out := make(map[string]bool, len(s))
	for _, v := range s {
		out[v] = true
	}
	return out
}

func (c *Conf) unmarshal(path string, dest any, tag string, flat bool) error {
	raw := c.Get(path)
	if path == "" {
		raw = c.data
	}
	if raw == nil {
		return nil
	}
	if flat {
		if m, ok := raw.(map[string]any); ok {
			raw = flatten(m, "", delim)
		}
	}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          tag,
		Result:           dest,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	})
	if err != nil {
		return err
	}
	return dec.Decode(raw)
}

func (c *Conf) lookup(path string) (any, bool) {
	if path == "" {
		return c.data, true
	}
	parts := strings.Split(path, delim)
	var cur any = c.data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func (c *Conf) merge(src map[string]any) {
	mergeMaps(c.data, src)
}

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				mergeMaps(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
}
