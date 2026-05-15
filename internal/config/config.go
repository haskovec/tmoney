// Package config provides persistent application settings for TMoney.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// maxRecentFiles is the maximum number of recent files to track.
const maxRecentFiles = 5

// saveDisabled gates Save() so that test runs don't pollute the real on-disk
// config. Defaults to true under `go test` (testing.Testing()), false in the
// production binary. Tests that exercise the persistence path must opt in via
// EnableSaveForTest.
var saveDisabled = testing.Testing()

// Config holds TMoney application settings persisted across sessions.
type Config struct {
	DefaultFile string   `json:"default_file,omitempty"`
	RecentFiles []string `json:"recent_files,omitempty"`
	LastFile    string   `json:"last_file,omitempty"`
	// Theme is the ID of the active theme (e.g. "turbo-vision",
	// "default", "light", or a user-installed theme stem). Empty
	// means "use the embedded default" — old config files without
	// this key keep working unchanged.
	Theme string `json:"theme,omitempty"`

	// ShowClosedPositions persists the "View → Show closed positions"
	// toggle. When true, investment views (dashboard cards, register
	// header, portfolio holdings list) include fully-sold securities
	// alongside currently-held ones so total-return numbers reflect the
	// account's full history. Old config files without this key keep the
	// default of false (closed positions hidden).
	ShowClosedPositions bool `json:"show_closed_positions,omitempty"`

	// path is the file path where this config is stored (not serialized).
	path string `json:"-"`
}

// Dir returns the configuration directory for TMoney.
// Follows the XDG Base Directory Specification:
//   - If $XDG_CONFIG_HOME is set, uses $XDG_CONFIG_HOME/tmoney
//   - Otherwise, uses $HOME/.config/tmoney (all platforms)
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tmoney"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tmoney"), nil
}

// Path returns the full path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk. If the file does not exist (first run),
// it returns a zero-value Config with no error.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(p)
}

// LoadFrom reads the config from the specified path. If the file does not
// exist, it returns a zero-value Config with no error.
func LoadFrom(path string) (*Config, error) {
	cfg := &Config{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.path = path
	return cfg, nil
}

// Save writes the config to disk atomically (write to .tmp, then rename).
// Skipped during `go test` to keep test runs from overwriting the user's
// on-disk config (e.g. setting LastFile to a temp path that vanishes after
// the test ends). See EnableSaveForTest for the opt-in escape hatch.
func (c *Config) Save() error {
	if saveDisabled {
		return nil
	}
	if c.path == "" {
		p, err := Path()
		if err != nil {
			return err
		}
		c.path = p
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// AddRecentFile adds a file path to the recent files list.
// It prepends the path, deduplicates, caps at maxRecentFiles, and updates LastFile.
func (c *Config) AddRecentFile(path string) {
	c.LastFile = path

	// Build new list with this path first
	recent := []string{path}
	for _, f := range c.RecentFiles {
		if f != path {
			recent = append(recent, f)
		}
	}

	// Cap at max
	if len(recent) > maxRecentFiles {
		recent = recent[:maxRecentFiles]
	}

	c.RecentFiles = recent
}

// EnableSaveForTest opts a single test in to actually persisting to disk by
// re-enabling Save() for the test's duration. Use this only in tests that
// directly exercise the on-disk format; everything else benefits from the
// default no-op Save() that protects the user's real config from test runs.
func EnableSaveForTest(t *testing.T) {
	t.Helper()
	prev := saveDisabled
	saveDisabled = false
	t.Cleanup(func() { saveDisabled = prev })
}

// ResolveDefaultFile returns the best file path to open.
// Priority: LastFile > DefaultFile > "" (caller handles final fallback).
func (c *Config) ResolveDefaultFile() string {
	if c.LastFile != "" {
		return c.LastFile
	}
	if c.DefaultFile != "" {
		return c.DefaultFile
	}
	return ""
}
