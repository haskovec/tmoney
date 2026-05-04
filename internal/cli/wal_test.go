package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWalColors_Sample(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors returned unexpected error: %v", err)
	}
	if got, want := wc.Special.Background, "#1d1f21"; got != want {
		t.Errorf("Special.Background = %q, want %q", got, want)
	}
	if got, want := wc.Special.Foreground, "#c5c8c6"; got != want {
		t.Errorf("Special.Foreground = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color0, "#1d1f21"; got != want {
		t.Errorf("Colors.Color0 = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color3, "#fabd2f"; got != want {
		t.Errorf("Colors.Color3 = %q, want %q", got, want)
	}
	if got, want := wc.Colors.Color15, "#ffffff"; got != want {
		t.Errorf("Colors.Color15 = %q, want %q", got, want)
	}
}

func TestReadWalColors_AllPaletteEntriesPopulated(t *testing.T) {
	wc, err := ReadWalColors(filepath.Join("testdata", "wal-sample-colors.json"))
	if err != nil {
		t.Fatalf("ReadWalColors returned unexpected error: %v", err)
	}
	entries := []struct {
		name string
		val  string
	}{
		{"Color0", wc.Colors.Color0},
		{"Color1", wc.Colors.Color1},
		{"Color2", wc.Colors.Color2},
		{"Color3", wc.Colors.Color3},
		{"Color4", wc.Colors.Color4},
		{"Color5", wc.Colors.Color5},
		{"Color6", wc.Colors.Color6},
		{"Color7", wc.Colors.Color7},
		{"Color8", wc.Colors.Color8},
		{"Color9", wc.Colors.Color9},
		{"Color10", wc.Colors.Color10},
		{"Color11", wc.Colors.Color11},
		{"Color12", wc.Colors.Color12},
		{"Color13", wc.Colors.Color13},
		{"Color14", wc.Colors.Color14},
		{"Color15", wc.Colors.Color15},
	}
	for _, e := range entries {
		if e.val == "" {
			t.Errorf("Colors.%s is empty after parsing fixture", e.name)
		}
	}
}

func TestReadWalColors_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist", "colors.json")
	_, err := ReadWalColors(missing)
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pywal cache not found at") {
		t.Errorf("error message should mention pywal cache; got %q", msg)
	}
	if !strings.Contains(msg, missing) {
		t.Errorf("error message should include the requested path; got %q", msg)
	}
	if !strings.Contains(msg, "is pywal installed and has it run?") {
		t.Errorf("error message should include the install/run hint; got %q", msg)
	}
}

func TestReadWalColors_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colors.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := ReadWalColors(path)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse pywal colors") {
		t.Errorf("error message should mention parse failure; got %q", err.Error())
	}
}
