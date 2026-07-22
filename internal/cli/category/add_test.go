package category_test

import (
	"bytes"
	"strings"
	"testing"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
)

func TestCategoryAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "add", "--name", "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("category add without --file should error")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected --file error, got: %v", err)
	}
}

func TestCategoryAdd_TopLevelDefaultType(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Groceries"}, stdout, stderr); err != nil {
		t.Fatalf("category add: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	cat, err := repo.GetByName("Groceries", nil)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cat.Type != categorydom.TypeExpense {
		t.Errorf("type = %s, want expense (default)", cat.Type)
	}
	if !cat.IsTopLevel() {
		t.Error("expected top-level category")
	}
}

func TestCategoryAdd_ExplicitType(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Side Gig", "--type", "income"}, stdout, stderr); err != nil {
		t.Fatalf("category add: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	cat, err := repo.GetByName("Side Gig", nil)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cat.Type != categorydom.TypeIncome {
		t.Errorf("type = %s, want income", cat.Type)
	}
}

func TestCategoryAdd_SubcategoryInheritsParentType(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Wages", categorydom.TypeIncome)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Overtime", "--parent", "Wages"}, stdout, stderr); err != nil {
		t.Fatalf("category add: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	parent, err := repo.GetByName("Wages", nil)
	if err != nil {
		t.Fatalf("reload parent: %v", err)
	}
	child, err := repo.GetByName("Overtime", &parent.ID)
	if err != nil {
		t.Fatalf("reload child: %v", err)
	}
	if child.Type != categorydom.TypeIncome {
		t.Errorf("child type = %s, want inherited income", child.Type)
	}
	if !strings.Contains(stdout.String(), "Wages") {
		t.Errorf("output should note the parent, got:\n%s", stdout.String())
	}
}

func TestCategoryAdd_DuplicateNameRefused(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Food"}, stdout, stderr)
	if err == nil {
		t.Fatal("adding a duplicate top-level name should be refused")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestCategoryAdd_UnknownParent(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Groceries", "--parent", "Nope"}, stdout, stderr)
	if err == nil {
		t.Fatal("adding under an unknown parent should error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestCategoryAdd_SubcategoryTypeMismatchRefused(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "add", "--file", path, "--name", "Groceries", "--parent", "Food", "--type", "income"}, stdout, stderr)
	if err == nil {
		t.Fatal("subcategory type mismatching the parent should be refused")
	}
	if !strings.Contains(err.Error(), "match") {
		t.Errorf("expected type-mismatch error, got: %v", err)
	}
}
