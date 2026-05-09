package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui"
)

// runTUI launches the interactive TUI mode for the given database file.
// Empty file means "use the last-opened file from config or the default
// location".
func runTUI(file string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	if file == "" {
		file = cfg.ResolveDefaultFile()
	}
	if file == "" {
		file = filepath.Join(db.DefaultDirectory(), "default.tdb")
	}

	var database *db.DB
	if _, statErr := os.Stat(file); os.IsNotExist(statErr) {
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		database, err = db.Create(file)
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
	} else {
		database, err = db.Open(file)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
	}
	defer database.Close()

	cfg.AddRecentFile(file)
	_ = cfg.Save()

	return tui.Run(database, cfg)
}
