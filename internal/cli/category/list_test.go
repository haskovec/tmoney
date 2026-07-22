package category_test

import (
	"bytes"
	"strings"
	"testing"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
)

func TestCategoryList_TreeOrdering(t *testing.T) {
	database, path := newCatFile(t)
	food := seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	seedChild(t, database, "Groceries", food, categorydom.TypeExpense)
	seedChild(t, database, "Dining Out", food, categorydom.TypeExpense)
	seedTopLevel(t, database, "Auto", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "list", "--file", path}, stdout, stderr); err != nil {
		t.Fatalf("category list: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()

	// Top-level alphabetical: Auto before Food.
	autoIdx := strings.Index(out, "Auto")
	foodIdx := strings.Index(out, "Food")
	if autoIdx < 0 || foodIdx < 0 || autoIdx > foodIdx {
		t.Errorf("top-level not alphabetical (Auto before Food):\n%s", out)
	}
	// Children alphabetical and after the parent: Dining Out before Groceries.
	diningIdx := strings.Index(out, "Dining Out")
	grocIdx := strings.Index(out, "Groceries")
	if diningIdx < foodIdx || diningIdx > grocIdx {
		t.Errorf("children not sorted/indented under Food:\n%s", out)
	}
	// Children are indented two spaces.
	if !strings.Contains(out, "  Groceries") {
		t.Errorf("expected two-space indent on child:\n%s", out)
	}
}

func TestCategoryList_TypeFilter(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	seedTopLevel(t, database, "Salary", categorydom.TypeIncome)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "list", "--file", path, "--type", "income"}, stdout, stderr); err != nil {
		t.Fatalf("category list --type income: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "Salary") {
		t.Errorf("income listing should include Salary:\n%s", out)
	}
	if strings.Contains(out, "Food") {
		t.Errorf("income listing should exclude expense Food:\n%s", out)
	}
}

func TestCategoryList_InvalidType(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "list", "--file", path, "--type", "nope"}, stdout, stderr)
	if err == nil {
		t.Fatal("invalid --type should error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("expected invalid-type error, got: %v", err)
	}
}

func TestCategoryList_ShowIDs(t *testing.T) {
	database, path := newCatFile(t)
	id := seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "list", "--file", path, "--show-ids"}, stdout, stderr); err != nil {
		t.Fatalf("category list --show-ids: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), id.String()) {
		t.Errorf("--show-ids output should contain the full UUID %s:\n%s", id, stdout.String())
	}
}

func TestCategoryList_SystemMarker(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()
	// The Value Adjustment system category is ensured on every open.

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "list", "--file", path}, stdout, stderr); err != nil {
		t.Fatalf("category list: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "Value Adjustment") || !strings.Contains(out, "[system]") {
		t.Errorf("expected the system category tagged [system]:\n%s", out)
	}
}
