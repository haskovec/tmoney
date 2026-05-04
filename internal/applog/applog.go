// Package applog provides an append-only plain-text log file used to
// surface non-fatal issues (theme parse warnings, etc.) for the user
// to inspect after the fact. Each entry is a single line prefixed with
// an RFC3339 timestamp and a category tag.
//
// The file lives at $XDG_CONFIG_HOME/tmoney/log.txt (or
// $HOME/.config/tmoney/log.txt) so it sits alongside config.json.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LogPath returns the absolute path of the tmoney log file. It mirrors
// the XDG resolution used by config.Path so both files live in the same
// directory.
func LogPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "log.txt"), nil
}

// Append writes one line to the log file: "<timestamp> [<category>] <message>\n".
// The parent directory is created on demand. The file is opened in
// append-only mode so concurrent writers don't truncate each other.
func Append(category, message string) error {
	path, err := LogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line := fmt.Sprintf("%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339), category, message)
	_, err = f.WriteString(line)
	return err
}

// configDir resolves the tmoney config directory the same way
// config.Dir does. Duplicated here to avoid an import cycle: the log
// package is meant to be importable from anywhere, including config.
func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tmoney"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tmoney"), nil
}
