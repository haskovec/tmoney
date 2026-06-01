package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionSubcommand_PrintsVersionInfo(t *testing.T) {
	origVersion, origBuildTime, origGitCommit := Version, BuildTime, GitCommit
	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()
	Version = "1.2.3"
	BuildTime = "2026-05-03T00:00:00Z"
	GitCommit = "abcdef1"

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := ExecuteWith([]string{"version"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(version) unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"tmoney 1.2.3", "2026-05-03T00:00:00Z", "abcdef1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected version output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestVersionSubcommand_NoExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"version", "extra"}, stdout, stderr)
	if err == nil {
		t.Errorf("expected error from `version extra`, got nil; stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}
