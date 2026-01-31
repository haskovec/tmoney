package main

import (
	"testing"
)

func TestRun_VersionFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"short version flag", []string{"-v"}},
		{"long version flag", []string{"--version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err != nil {
				t.Errorf("run(%v) returned error: %v", tt.args, err)
			}
		})
	}
}

func TestRun_HelpFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"short help flag", []string{"-h"}},
		{"long help flag", []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err != nil {
				t.Errorf("run(%v) returned error: %v", tt.args, err)
			}
		})
	}
}

func TestRun_NoArgs(t *testing.T) {
	err := run([]string{})
	if err != nil {
		t.Errorf("run([]) returned error: %v", err)
	}
}

func TestRun_UnknownArgs(t *testing.T) {
	// Currently unknown args are silently ignored and we fall through to TUI mode
	// This tests that behavior doesn't cause errors
	err := run([]string{"some-file.tdb"})
	if err != nil {
		t.Errorf("run with file argument returned error: %v", err)
	}
}
