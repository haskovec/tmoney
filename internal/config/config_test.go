package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadFrom() returned nil config")
	}
	if cfg.DefaultFile != "" {
		t.Errorf("DefaultFile = %q, want empty", cfg.DefaultFile)
	}
	if cfg.LastFile != "" {
		t.Errorf("LastFile = %q, want empty", cfg.LastFile)
	}
	if len(cfg.RecentFiles) != 0 {
		t.Errorf("RecentFiles = %v, want empty", cfg.RecentFiles)
	}
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	EnableSaveForTest(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		DefaultFile: "/home/user/test.tdb",
		RecentFiles: []string{"/home/user/test.tdb", "/home/user/other.tdb"},
		LastFile:    "/home/user/test.tdb",
		path:        path,
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load it back
	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}

	if loaded.DefaultFile != cfg.DefaultFile {
		t.Errorf("DefaultFile = %q, want %q", loaded.DefaultFile, cfg.DefaultFile)
	}
	if loaded.LastFile != cfg.LastFile {
		t.Errorf("LastFile = %q, want %q", loaded.LastFile, cfg.LastFile)
	}
	if len(loaded.RecentFiles) != 2 {
		t.Fatalf("RecentFiles length = %d, want 2", len(loaded.RecentFiles))
	}
	if loaded.RecentFiles[0] != "/home/user/test.tdb" {
		t.Errorf("RecentFiles[0] = %q, want %q", loaded.RecentFiles[0], "/home/user/test.tdb")
	}
	if loaded.RecentFiles[1] != "/home/user/other.tdb" {
		t.Errorf("RecentFiles[1] = %q, want %q", loaded.RecentFiles[1], "/home/user/other.tdb")
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	EnableSaveForTest(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.json")

	cfg := &Config{path: path}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created in subdirectory")
	}
}

// TestSave_SkippedUnderGoTest documents that Save() is a no-op when running
// inside `go test`. This prevents the test suite from polluting the user's
// real config (e.g. setting LastFile to a temp .tdb that vanishes after the
// test ends).
func TestSave_SkippedUnderGoTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{LastFile: "/tmp/should-not-be-saved.tdb", path: path}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config file should not have been written under go test, stat err = %v", err)
	}
}

func TestAddRecentFile_Prepend(t *testing.T) {
	cfg := &Config{
		RecentFiles: []string{"/a.tdb", "/b.tdb"},
	}

	cfg.AddRecentFile("/c.tdb")

	if cfg.LastFile != "/c.tdb" {
		t.Errorf("LastFile = %q, want %q", cfg.LastFile, "/c.tdb")
	}
	if len(cfg.RecentFiles) != 3 {
		t.Fatalf("RecentFiles length = %d, want 3", len(cfg.RecentFiles))
	}
	if cfg.RecentFiles[0] != "/c.tdb" {
		t.Errorf("RecentFiles[0] = %q, want %q", cfg.RecentFiles[0], "/c.tdb")
	}
	if cfg.RecentFiles[1] != "/a.tdb" {
		t.Errorf("RecentFiles[1] = %q, want %q", cfg.RecentFiles[1], "/a.tdb")
	}
}

func TestAddRecentFile_Deduplicates(t *testing.T) {
	cfg := &Config{
		RecentFiles: []string{"/a.tdb", "/b.tdb", "/c.tdb"},
	}

	cfg.AddRecentFile("/b.tdb")

	if len(cfg.RecentFiles) != 3 {
		t.Fatalf("RecentFiles length = %d, want 3", len(cfg.RecentFiles))
	}
	if cfg.RecentFiles[0] != "/b.tdb" {
		t.Errorf("RecentFiles[0] = %q, want %q (should be moved to front)", cfg.RecentFiles[0], "/b.tdb")
	}
	if cfg.RecentFiles[1] != "/a.tdb" {
		t.Errorf("RecentFiles[1] = %q, want %q", cfg.RecentFiles[1], "/a.tdb")
	}
	if cfg.RecentFiles[2] != "/c.tdb" {
		t.Errorf("RecentFiles[2] = %q, want %q", cfg.RecentFiles[2], "/c.tdb")
	}
}

func TestAddRecentFile_CapsAtMax(t *testing.T) {
	cfg := &Config{}

	// Add 7 files, expect only 5 retained
	for i := range 7 {
		cfg.AddRecentFile(filepath.Join("/tmp", string(rune('a'+i))+".tdb"))
	}

	if len(cfg.RecentFiles) != maxRecentFiles {
		t.Errorf("RecentFiles length = %d, want %d", len(cfg.RecentFiles), maxRecentFiles)
	}
	// Most recent should be first
	want := filepath.Join("/tmp", "g.tdb")
	if cfg.RecentFiles[0] != want {
		t.Errorf("RecentFiles[0] = %q, want %q", cfg.RecentFiles[0], want)
	}
}

func TestAddRecentFile_SetsLastFile(t *testing.T) {
	cfg := &Config{}

	cfg.AddRecentFile("/test.tdb")

	if cfg.LastFile != "/test.tdb" {
		t.Errorf("LastFile = %q, want %q", cfg.LastFile, "/test.tdb")
	}
}

func TestResolveDefaultFile_LastFile(t *testing.T) {
	cfg := &Config{
		DefaultFile: "/default.tdb",
		LastFile:    "/last.tdb",
	}

	result := cfg.ResolveDefaultFile()
	if result != "/last.tdb" {
		t.Errorf("ResolveDefaultFile() = %q, want %q (LastFile has priority)", result, "/last.tdb")
	}
}

func TestResolveDefaultFile_DefaultFile(t *testing.T) {
	cfg := &Config{
		DefaultFile: "/default.tdb",
	}

	result := cfg.ResolveDefaultFile()
	if result != "/default.tdb" {
		t.Errorf("ResolveDefaultFile() = %q, want %q", result, "/default.tdb")
	}
}

func TestResolveDefaultFile_Empty(t *testing.T) {
	cfg := &Config{}

	result := cfg.ResolveDefaultFile()
	if result != "" {
		t.Errorf("ResolveDefaultFile() = %q, want empty", result)
	}
}

func TestDir(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if dir == "" {
		t.Error("Dir() returned empty string")
	}
	if filepath.Base(dir) != "tmoney" {
		t.Errorf("Dir() = %q, want directory named 'tmoney'", dir)
	}
}

func TestDir_XDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	want := filepath.Join(tmp, "tmoney")
	if dir != want {
		t.Errorf("Dir() = %q, want %q (should respect $XDG_CONFIG_HOME)", dir, want)
	}
}

func TestDir_FallbackDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error: %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	want := filepath.Join(home, ".config", "tmoney")
	if dir != want {
		t.Errorf("Dir() = %q, want %q (should fall back to $HOME/.config/tmoney)", dir, want)
	}
}

func TestPath(t *testing.T) {
	p, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if p == "" {
		t.Error("Path() returned empty string")
	}
	if filepath.Base(p) != "config.json" {
		t.Errorf("Path() = %q, want file named 'config.json'", p)
	}
}
