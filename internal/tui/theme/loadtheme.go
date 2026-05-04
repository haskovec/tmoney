package theme

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// LoadTheme returns the parsed theme for the given ID. The user theme
// directory (UserThemesDir) is consulted first: a `<id>.toml` file
// there shadows the embedded built-in of the same ID, so users can
// override "default", "light", or "turbo-vision" with their own
// variants without inventing a new ID. If the user dir has no matching
// file, the embedded built-in is returned via LoadBuiltin. An ID
// present in neither location surfaces as an error so callers can fall
// back to the embedded default.
//
// A user file that exists but fails to parse propagates the parse
// error rather than silently shadowing it with the built-in — the user
// put the file there deliberately and a regression in their theme
// should be visible (Phase 9 surfaces issues as a status-bar toast).
func LoadTheme(id string) (*Theme, []Issue, error) {
	dir, err := UserThemesDir()
	if err == nil {
		path := filepath.Join(dir, id+".toml")
		if _, statErr := os.Stat(path); statErr == nil {
			return ParseFromFile(path)
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return nil, nil, statErr
		}
	}
	return LoadBuiltin(id)
}
