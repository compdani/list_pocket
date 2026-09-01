package main

import (
	"fmt"
	"os"

	"github.com/compdani/list_pocket/internal/assets"
)

func newConfigFile(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("error creating %s: %v", path, err)
	}

	// Initialize the static file system into which all
	// required static assets (.sql, .js files etc.) are loaded.
	fsys := initFS(appDir, "", "", "")
	b, err := assets.ReadFile(fsys, "config.toml.sample")
	if err != nil {
		return fmt.Errorf("error reading sample config: %v", err)
	}

	return os.WriteFile(path, b, 0644)
}
