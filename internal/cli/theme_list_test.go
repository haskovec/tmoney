package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writeUserTheme drops a tiny user theme file at $XDG_CONFIG_HOME/tmoney/themes/<id>.toml
// containing just `name = "<displayName>"`. That's enough for the listing
// command to pick it up via DiscoverUserThemes and report its NAME.
func writeUserTheme(t *testing.T, configRoot, id, displayName string) {
	t.Helper()
	dir := filepath.Join(configRoot, "tmoney", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: mkdir %s: %v", dir, err)
	}
	body := []byte("name = \"" + displayName + "\"\n")
	if err := os.WriteFile(filepath.Join(dir, id+".toml"), body, 0o644); err != nil {
		t.Fatalf("setup: write user theme: %v", err)
	}
}

// writeConfigTheme writes config.json with the given active theme so
// the list command picks it up.
func writeConfigTheme(t *testing.T, configRoot, themeID string) {
	t.Helper()
	dir := filepath.Join(configRoot, "tmoney")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: mkdir %s: %v", dir, err)
	}
	body := []byte("{\"theme\":\"" + themeID + "\"}\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), body, 0o644); err != nil {
		t.Fatalf("setup: write config: %v", err)
	}
}

func TestThemeList_PrintsHeaderAndAllThemes(t *testing.T) {
	configRoot := t.TempDir()
	writeUserTheme(t, configRoot, "mine", "My Theme")
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(theme list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"ID", "SOURCE", "NAME", "ACTIVE"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected header column %q in output, got:\n%s", want, out)
		}
	}
	for _, want := range []string{"default", "light", "turbo-vision", "mine"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected theme ID %q in output, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "My Theme") {
		t.Errorf("expected user theme NAME 'My Theme' in output, got:\n%s", out)
	}
}

func TestThemeList_BuiltinAndUserSources(t *testing.T) {
	configRoot := t.TempDir()
	writeUserTheme(t, configRoot, "mine", "My Theme")
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}

	// Match each row by ID and check its SOURCE column.
	wantSource := map[string]string{
		"default":      "built-in",
		"light":        "built-in",
		"turbo-vision": "built-in",
		"mine":         "user",
	}
	for id, source := range wantSource {
		// Look for a line beginning with the ID (possibly followed by
		// padding whitespace from tabwriter) and the expected source.
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(id) + `\s+` + regexp.QuoteMeta(source) + `\b`)
		if !re.MatchString(stdout.String()) {
			t.Errorf("expected row for %q with source %q; got:\n%s", id, source, stdout.String())
		}
	}
}

func TestThemeList_UserOverrideOfBuiltinShowsAsUser(t *testing.T) {
	// A user file named default.toml shadows the embedded default theme.
	// The list should report that ID with source "user", not "built-in",
	// and use the user theme's name.
	configRoot := t.TempDir()
	writeUserTheme(t, configRoot, "default", "User Override")
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}

	re := regexp.MustCompile(`(?m)^default\s+user\s+User Override\b`)
	if !re.MatchString(stdout.String()) {
		t.Errorf("expected default row to be sourced 'user' with name 'User Override'; got:\n%s", stdout.String())
	}
	// And the ID must not appear twice (no duplicate row from built-in).
	if got := strings.Count(stdout.String(), "\ndefault "); got > 1 {
		t.Errorf("expected exactly one default row, got %d; output:\n%s", got, stdout.String())
	}
}

func TestThemeList_ActiveThemeMarkedWithStar(t *testing.T) {
	configRoot := t.TempDir()
	writeConfigTheme(t, configRoot, "turbo-vision")
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	// turbo-vision row should end with `*`; default and light should not.
	starRow := regexp.MustCompile(`(?m)^turbo-vision\s+built-in\s+\S.*\*\s*$`)
	if !starRow.MatchString(out) {
		t.Errorf("expected turbo-vision row to be marked active with '*'; got:\n%s", out)
	}
	plainRow := regexp.MustCompile(`(?m)^default\s+built-in\s+\S[^*]*$`)
	if !plainRow.MatchString(out) {
		t.Errorf("expected default row to NOT be marked active; got:\n%s", out)
	}
}

func TestThemeList_NoActiveThemeInConfig_NoStarShown(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}
	if strings.Contains(stdout.String(), "*") {
		t.Errorf("expected no active marker when config has no theme; got:\n%s", stdout.String())
	}
}

func TestThemeList_RowsSortedByID(t *testing.T) {
	configRoot := t.TempDir()
	writeUserTheme(t, configRoot, "zzz-late", "Late")
	writeUserTheme(t, configRoot, "aaa-early", "Early")
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith: %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	indices := []struct {
		id  string
		idx int
	}{
		{"aaa-early", strings.Index(out, "\naaa-early")},
		{"default", strings.Index(out, "\ndefault")},
		{"light", strings.Index(out, "\nlight")},
		{"mine", strings.Index(out, "\nmine")},
		{"turbo-vision", strings.Index(out, "\nturbo-vision")},
		{"zzz-late", strings.Index(out, "\nzzz-late")},
	}
	for i := 1; i < len(indices); i++ {
		if indices[i].idx == -1 {
			continue
		}
		if indices[i-1].idx != -1 && indices[i].idx < indices[i-1].idx {
			t.Errorf("expected %q to appear after %q in output; output:\n%s",
				indices[i].id, indices[i-1].id, out)
		}
	}
}

func TestThemeList_HelpListsCommand(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"theme", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "List") {
		t.Errorf("expected `theme list --help` output to describe the command; got:\n%s", stdout.String())
	}
}
