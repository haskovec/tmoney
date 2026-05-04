package theme

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

// builtinFS holds the TOML files for every theme that ships in the
// binary. Adding a new built-in theme is a matter of dropping its
// `<id>.toml` into the `themes/` directory — `BuiltinIDs` will pick
// it up automatically on the next build.
//
//go:embed themes/*.toml
var builtinFS embed.FS

// builtinDir is the path inside builtinFS where the embedded TOML
// files live. Centralized so file-name lookups can't drift.
const builtinDir = "themes"

// BuiltinIDs returns the sorted list of theme IDs compiled into the
// binary. The ID is the filename stem (e.g. "turbo-vision.toml" ->
// "turbo-vision"). The result is freshly allocated; callers may
// mutate it.
func BuiltinIDs() []string {
	entries, err := builtinFS.ReadDir(builtinDir)
	if err != nil {
		// builtinFS is constructed at compile time from a known
		// directory; a read error here would mean a build-time
		// misconfiguration and should never happen at runtime.
		return nil
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
	return ids
}

// LoadBuiltin returns the parsed theme for the given built-in ID,
// along with any non-fatal Issues found while parsing. An unknown ID
// produces an error; otherwise the embedded TOML is fed through the
// regular `Parse` machinery so built-ins receive the same validation
// and default-fallback treatment as user themes.
func LoadBuiltin(id string) (*Theme, []Issue, error) {
	data, err := builtinFS.ReadFile(builtinDir + "/" + id + ".toml")
	if err != nil {
		return nil, nil, fmt.Errorf("unknown built-in theme %q", id)
	}
	t, issues, err := Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse built-in theme %q: %w", id, err)
	}
	if t.Name == "" {
		t.Name = id
	}
	return t, issues, nil
}
