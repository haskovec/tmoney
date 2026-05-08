package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPriceCurrent_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "current", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price current AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestPriceCurrent_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "current", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price current) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestPriceCurrent_SecurityNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "current", "ZZZZ", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price current ZZZZ) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestPriceCurrent_ShowsMostRecent(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "current", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price current AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"CURRENT PRICE: AAPL",
		"Apple Inc.",
		"2024-03-15",
		"170.25",
		"Import",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestPriceCurrent_NoPriceExists(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "current", "AAPL", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price current AAPL) with no prices should return error")
	}
	if !strings.Contains(err.Error(), "no price found") {
		t.Errorf("error should mention no price found, got: %v", err)
	}
}

func TestPriceCurrent_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "current", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price current AAPL EXTRA) should return error")
	}
}

func TestPriceCurrent_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "current", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price current --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "current") {
		t.Errorf("expected `price current --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestPriceCmd_HelpListsCurrent(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "current") {
		t.Errorf("expected `price --help` to list `current`; got:\n%s", stdout.String())
	}
}
