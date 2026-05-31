package price_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
)

func TestPriceUpdate_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "update", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price update --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "update") {
		t.Errorf("expected `price update --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestPriceCmd_HelpListsUpdate(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "update") {
		t.Errorf("expected `price --help` to list `update`; got:\n%s", stdout.String())
	}
}
