package theme

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UserThemesDir returns the directory where user-authored theme files
// live. Follows the XDG Base Directory Specification, mirroring
// config.Dir(): if $XDG_CONFIG_HOME is set, returns
// $XDG_CONFIG_HOME/tmoney/themes; otherwise $HOME/.config/tmoney/themes.
//
// The returned path is not guaranteed to exist; callers that need to
// distinguish "no themes" from "no directory" should stat it
// themselves. DiscoverUserThemes treats a missing directory as an
// empty list.
func UserThemesDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tmoney", "themes"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tmoney", "themes"), nil
}

// DiscoverUserThemes returns the sorted list of theme IDs found in the
// user theme directory. Each ID is the filename stem of a `.toml` file
// in UserThemesDir() (e.g. "wal.toml" -> "wal"). A missing directory
// is not an error and returns an empty slice — first-run users have no
// themes installed.
//
// Subdirectories and non-`.toml` files are skipped silently. The
// returned slice is freshly allocated; callers may mutate it.
func DiscoverUserThemes() ([]string, error) {
	dir, err := UserThemesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".toml"))
	}
	sort.Strings(ids)
	return ids, nil
}
