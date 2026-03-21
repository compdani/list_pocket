package main

import (
	"fmt"
	"os"
)

func newConfigFile(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("error creating %s: %v", path, err)
	}

	// Initialize the static file system into which all
	// required static assets (.sql, .js files etc.) are loaded.
	fs := initFS(appDir, "", "", "")
	b, err := fs.Read("config.toml.sample")
	if err != nil {
		return fmt.Errorf("error reading sample config (is binary stuffed?): %v", err)
	}

	return os.WriteFile(path, b, 0644)
}
