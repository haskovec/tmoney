package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/types"
)

func makeTestAccounts() []*account.Account {
	return []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking", Active: true},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Visa", Active: true},
	}
}

func TestBuildImportOptionsDialog(t *testing.T) {
	t.Run("title and visibility", func(t *testing.T) {
		d, _ := buildImportOptionsDialog(makeTestAccounts(), types.ID{})
		if d.Title() != "Import Transactions" {
			t.Errorf("title = %q, want 'Import Transactions'", d.Title())
		}
		if !d.IsVisible() {
			t.Error("dialog should be visible after creation")
		}
	})

	t.Run("returns one ID per account", func(t *testing.T) {
		accounts := makeTestAccounts()
		_, ids := buildImportOptionsDialog(accounts, types.ID{})
		if len(ids) != len(accounts) {
			t.Fatalf("got %d IDs, want %d", len(ids), len(accounts))
		}
		for i, a := range accounts {
			if ids[i] != a.ID {
				t.Errorf("ids[%d] = %v, want %v", i, ids[i], a.ID)
			}
		}
	})

	t.Run("default account is preselected", func(t *testing.T) {
		accounts := makeTestAccounts()
		d, _ := buildImportOptionsDialog(accounts, accounts[1].ID)
		fields := d.Fields()
		if fields[1].SelectedIndex != 1 {
			t.Errorf("default selection = %d, want 1", fields[1].SelectedIndex)
		}
	})

	t.Run("file path field is required", func(t *testing.T) {
		d, _ := buildImportOptionsDialog(makeTestAccounts(), types.ID{})
		fields := d.Fields()
		if !fields[0].Required {
			t.Error("file path field should be required")
		}
	})

	t.Run("format and duplicate fields offer expected options", func(t *testing.T) {
		d, _ := buildImportOptionsDialog(makeTestAccounts(), types.ID{})
		fields := d.Fields()
		if got := fields[2].Options; len(got) != len(importFormatOptions) {
			t.Errorf("format options len = %d, want %d", len(got), len(importFormatOptions))
		}
		if got := fields[3].Options; len(got) != len(importDuplicateOptions) {
			t.Errorf("dup options len = %d, want %d", len(got), len(importDuplicateOptions))
		}
	})

	t.Run("placeholder when no accounts", func(t *testing.T) {
		d, ids := buildImportOptionsDialog(nil, types.ID{})
		if len(ids) != 0 {
			t.Errorf("ids = %d, want 0", len(ids))
		}
		fields := d.Fields()
		if fields[1].Options[0] != "(no accounts)" {
			t.Errorf("expected placeholder '(no accounts)', got %q", fields[1].Options[0])
		}
	})
}

func TestBuildImportConfirmDialog(t *testing.T) {
	state := &importDialogState{
		filePath:    "/tmp/test.qif",
		accountName: "Checking",
		preview: &imexport.ImportResult{
			Rows: []imexport.ImportRow{{
				Record: &imexport.ImportRecord{
					Date:   types.MustParseDate("2024-01-15"),
					Amount: types.MustNewMoney("-12.34"),
				},
				Action: imexport.ImportActionNew,
			}},
			DateFrom: types.MustParseDate("2024-01-01"),
			DateTo:   types.MustParseDate("2024-01-31"),
		},
	}

	t.Run("title and visibility", func(t *testing.T) {
		d := buildImportConfirmDialog(state)
		if d.Title() != "Confirm Import" {
			t.Errorf("title = %q, want 'Confirm Import'", d.Title())
		}
		if !d.IsVisible() {
			t.Error("dialog should be visible after creation")
		}
	})

	t.Run("primary button is Import when there are rows", func(t *testing.T) {
		d := buildImportConfirmDialog(state)
		btns := d.Buttons()
		var primary string
		for _, b := range btns {
			if b.Primary {
				primary = b.Label
			}
		}
		if primary != "Import" {
			t.Errorf("primary button = %q, want 'Import'", primary)
		}
	})

	t.Run("primary button is Close when no rows parsed", func(t *testing.T) {
		empty := &importDialogState{
			filePath:    "/tmp/empty.qif",
			accountName: "Checking",
			preview:     &imexport.ImportResult{},
		}
		d := buildImportConfirmDialog(empty)
		var primary string
		for _, b := range d.Buttons() {
			if b.Primary {
				primary = b.Label
			}
		}
		if primary != "Close" {
			t.Errorf("primary button = %q, want 'Close'", primary)
		}
	})

	t.Run("focus defaults to primary button", func(t *testing.T) {
		d := buildImportConfirmDialog(state)
		// Primary button is the first button, so focus index should be
		// fields_count (the first focusable past the fields is the primary).
		want := len(d.Fields())
		if d.FocusIndex() != want {
			t.Errorf("focusIndex = %d, want %d", d.FocusIndex(), want)
		}
	})
}

func TestImportFormatFromIndex(t *testing.T) {
	cases := []struct {
		idx  int
		want imexport.Format
	}{
		{0, ""},
		{1, imexport.FormatCSV},
		{2, imexport.FormatQIF},
		{3, imexport.FormatOFX},
		{99, ""},
	}
	for _, c := range cases {
		if got := importFormatFromIndex(c.idx); got != c.want {
			t.Errorf("importFormatFromIndex(%d) = %q, want %q", c.idx, got, c.want)
		}
	}
}

func TestBuildImportSourcePickerDialog(t *testing.T) {
	sources := []string{"Checking", "Savings", "Visa"}
	d := buildImportSourcePickerDialog(sources, "tmoney Checking")

	if d.Title() != "Pick Source Account" {
		t.Errorf("title = %q", d.Title())
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(fields))
	}
	if fields[0].Value != "tmoney Checking" {
		t.Errorf("target field = %q, want 'tmoney Checking'", fields[0].Value)
	}
	if got := fields[2].Options; len(got) != 3 || got[0] != "Checking" {
		t.Errorf("source options = %v", got)
	}

	var primary string
	for _, b := range d.Buttons() {
		if b.Primary {
			primary = b.Label
		}
	}
	if primary != "Continue" {
		t.Errorf("primary button = %q, want 'Continue'", primary)
	}
}

func TestImportDuplicateFromIndex(t *testing.T) {
	cases := []struct {
		idx  int
		want imexport.DuplicateHandling
	}{
		{0, imexport.DuplicateHandlingNone},
		{1, imexport.DuplicateHandlingSkip},
		{2, imexport.DuplicateHandlingUpdate},
		{99, imexport.DuplicateHandlingNone},
	}
	for _, c := range cases {
		if got := importDuplicateFromIndex(c.idx); got != c.want {
			t.Errorf("importDuplicateFromIndex(%d) = %q, want %q", c.idx, got, c.want)
		}
	}
}
