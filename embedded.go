package listpocket

import "embed"

// Files holds static templates, i18n packs, the sample config, and permissions.
//
//go:embed all:static all:i18n config.toml.sample permissions.json
var Files embed.FS

// PluginDocs is the plugin developer guide, for the AI assistant.
//
//go:embed docs/docs/content/plugins.md
var PluginDocs string
