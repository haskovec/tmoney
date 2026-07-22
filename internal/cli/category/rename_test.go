package category_test

import (
	"bytes"
	"strings"
	"testing"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
)

func TestCategoryRename_ByName(t *testing.T) {
	database, path := newCatFile(t)
	id := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--name", "Groceries", "--to", "Food & Groceries"}, stdout, stderr); err != nil {
		t.Fatalf("category rename: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	cat, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cat.Name != "Food & Groceries" {
		t.Errorf("name = %q, want %q", cat.Name, "Food & Groceries")
	}
}

func TestCategoryRename_ByID(t *testing.T) {
	database, path := newCatFile(t)
	id := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--id", id.String(), "--to", "Utilities"}, stdout, stderr); err != nil {
		t.Fatalf("category rename --id: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	cat, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cat.Name != "Utilities" {
		t.Errorf("name = %q, want Utilities", cat.Name)
	}
}

func TestCategoryRename_ExclusivityBothRefused(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--id", "x", "--name", "y", "--to", "z"}, stdout, stderr)
	if err == nil {
		t.Fatal("passing both --id and --name should error")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("expected exactly-one error, got: %v", err)
	}
}

func TestCategoryRename_NeitherRefused(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--to", "z"}, stdout, stderr)
	if err == nil {
		t.Fatal("passing neither --id nor --name should error")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("expected exactly-one error, got: %v", err)
	}
}

func TestCategoryRename_MissingTo(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--name", "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("rename without --to should error")
	}
	if !strings.Contains(err.Error(), "--to") {
		t.Errorf("expected --to error, got: %v", err)
	}
}

func TestCategoryRename_SystemRefused(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()
	// Value Adjustment system category is ensured on open.

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "rename", "--file", path, "--name", "Value Adjustment", "--to", "Nope"}, stdout, stderr)
	if err == nil {
		t.Fatal("renaming a system category should be refused")
	}
	if !strings.Contains(err.Error(), "system category") {
		t.Errorf("expected system-category refusal, got: %v", err)
	}
}
