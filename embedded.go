package listpocket

import "embed"

// Files holds static templates, i18n packs, the sample config, and permissions.
//
//go:embed all:static all:i18n config.toml.sample permissions.json
var Files embed.FS
