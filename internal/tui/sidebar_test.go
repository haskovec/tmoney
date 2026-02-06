package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// testAccount creates an account with the given name and type for testing.
func testAccount(name string, accountType models.AccountType) *models.Account {
	return &models.Account{
		BaseModel:      models.BaseModel{ID: models.NewID(), CreatedAt: models.NewTimestamp(time.Now()), UpdatedAt: models.NewTimestamp(time.Now())},
		Name:           name,
		Type:           accountType,
		Currency:       "USD",
		OpeningBalance: models.ZeroMoney,
		OpeningDate:    models.Today(),
		Active:         true,
	}
}

func TestNewSidebar(t *testing.T) {
	s := NewSidebar()
	if s == nil {
		t.Fatal("NewSidebar() returned nil")
	}
	if !s.focused {
		t.Error("sidebar should be focused by default")
	}
	if len(s.items) != 0 {
		t.Error("sidebar should start with no items")
	}
	if s.cursor != 0 {
		t.Error("cursor should start at 0")
	}
	if !s.selectedAccountID.IsNil() {
		t.Error("no account should be selected initially")
	}
}

func TestSidebar_SetAccounts_EmptyList(t *testing.T) {
	s := NewSidebar()
	s.SetAccounts(nil, nil)

	if len(s.items) != 0 {
		t.Errorf("expected 0 items, got %d", len(s.items))
	}
}

func TestSidebar_SetAccounts_SingleGroup(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
	}

	s.SetAccounts(accounts, nil)

	// Checking and Savings share the "Bank Accounts" group
	// Expect: 1 group header + 2 accounts = 3 items
	if len(s.items) != 3 {
		t.Errorf("expected 3 items (1 group + 2 accounts), got %d", len(s.items))
	}

	if s.items[0].kind != sidebarItemGroup {
		t.Error("first item should be a group header")
	}
	if s.items[0].groupKey != "Bank Accounts" {
		t.Errorf("group key = %q, want %q", s.items[0].groupKey, "Bank Accounts")
	}
	if s.items[1].kind != sidebarItemAccount {
		t.Error("second item should be an account")
	}
	if s.items[1].account.Name != "Checking" {
		t.Errorf("second item name = %q, want %q", s.items[1].account.Name, "Checking")
	}
}

func TestSidebar_SetAccounts_MultipleGroups(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Visa", models.AccountTypeCreditCard),
		testAccount("Brokerage", models.AccountTypeInvestment),
	}

	s.SetAccounts(accounts, nil)

	// 3 groups, each with 1 account = 6 items
	if len(s.items) != 6 {
		t.Errorf("expected 6 items (3 groups + 3 accounts), got %d", len(s.items))
	}

	// Verify group order: Bank Accounts, Credit Cards, Investments
	groupLabels := []string{}
	for _, item := range s.items {
		if item.kind == sidebarItemGroup {
			groupLabels = append(groupLabels, item.groupKey)
		}
	}
	expectedOrder := []string{"Bank Accounts", "Credit Cards", "Investments"}
	if len(groupLabels) != len(expectedOrder) {
		t.Fatalf("expected %d groups, got %d", len(expectedOrder), len(groupLabels))
	}
	for i, label := range groupLabels {
		if label != expectedOrder[i] {
			t.Errorf("group[%d] = %q, want %q", i, label, expectedOrder[i])
		}
	}
}

func TestSidebar_MoveUpDown(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings]

	if s.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", s.cursor)
	}

	s.MoveDown()
	if s.cursor != 1 {
		t.Errorf("after MoveDown, cursor = %d, want 1", s.cursor)
	}

	s.MoveDown()
	if s.cursor != 2 {
		t.Errorf("after 2x MoveDown, cursor = %d, want 2", s.cursor)
	}

	// Should not go past last item
	s.MoveDown()
	if s.cursor != 2 {
		t.Errorf("cursor should stay at 2 (last item), got %d", s.cursor)
	}

	s.MoveUp()
	if s.cursor != 1 {
		t.Errorf("after MoveUp, cursor = %d, want 1", s.cursor)
	}

	s.MoveUp()
	if s.cursor != 0 {
		t.Errorf("after 2x MoveUp, cursor = %d, want 0", s.cursor)
	}

	// Should not go before first item
	s.MoveUp()
	if s.cursor != 0 {
		t.Errorf("cursor should stay at 0 (first item), got %d", s.cursor)
	}
}

func TestSidebar_CollapseExpand(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings] -> 3 items

	// Cursor is on group header "Bank Accounts"
	s.CollapseGroup()

	if len(s.items) != 1 {
		t.Errorf("after collapse, expected 1 item (group header only), got %d", len(s.items))
	}

	// Expand it
	s.ExpandGroup()

	if len(s.items) != 3 {
		t.Errorf("after expand, expected 3 items, got %d", len(s.items))
	}
}

func TestSidebar_CollapseFromAccount(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
	}
	s.SetAccounts(accounts, nil)

	// Move to account "Checking"
	s.MoveDown()
	if s.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", s.cursor)
	}

	// Left arrow on an account should move cursor to group header
	s.CollapseGroup()

	if s.cursor != 0 {
		t.Errorf("cursor should move to group header (0), got %d", s.cursor)
	}
	// Items should still show (not collapsed yet)
	if len(s.items) != 3 {
		t.Errorf("items should still be 3, got %d", len(s.items))
	}
}

func TestSidebar_ToggleCollapse(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking]

	// Toggle collapse (on group header)
	s.ToggleCollapse()
	if len(s.items) != 1 {
		t.Errorf("after toggle, expected 1 item, got %d", len(s.items))
	}

	// Toggle again to expand
	s.ToggleCollapse()
	if len(s.items) != 2 {
		t.Errorf("after second toggle, expected 2 items, got %d", len(s.items))
	}
}

func TestSidebar_Select_Account(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", models.AccountTypeChecking)
	accounts := []*models.Account{checking}
	s.SetAccounts(accounts, nil)

	// Move to account
	s.MoveDown()

	// Select it
	selected := s.Select()
	if !selected {
		t.Error("Select() should return true for account item")
	}
	if s.selectedAccountID != checking.ID {
		t.Errorf("selectedAccountID = %v, want %v", s.selectedAccountID, checking.ID)
	}
}

func TestSidebar_Select_Group(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
	}
	s.SetAccounts(accounts, nil)

	// Cursor is on group header
	selected := s.Select()
	if selected {
		t.Error("Select() should return false for group item (toggles collapse instead)")
	}

	// Group should be collapsed now
	if len(s.items) != 1 {
		t.Errorf("expected 1 item after selecting group (collapsed), got %d", len(s.items))
	}
}

func TestSidebar_SelectedAccount(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", models.AccountTypeChecking)
	accounts := []*models.Account{checking}
	s.SetAccounts(accounts, nil)

	// No selection yet
	if s.SelectedAccount() != nil {
		t.Error("SelectedAccount() should be nil before any selection")
	}

	// Select the account
	s.MoveDown()
	s.Select()

	acct := s.SelectedAccount()
	if acct == nil {
		t.Fatal("SelectedAccount() should not be nil after selection")
	}
	if acct.Name != "Checking" {
		t.Errorf("SelectedAccount().Name = %q, want %q", acct.Name, "Checking")
	}
}

func TestSidebar_CursorItem(t *testing.T) {
	s := NewSidebar()

	// Empty sidebar
	if s.CursorItem() != nil {
		t.Error("CursorItem() should be nil for empty sidebar")
	}

	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
	}
	s.SetAccounts(accounts, nil)

	item := s.CursorItem()
	if item == nil {
		t.Fatal("CursorItem() should not be nil")
	}
	if item.kind != sidebarItemGroup {
		t.Error("first CursorItem should be a group")
	}
}

func TestSidebar_Focus(t *testing.T) {
	s := NewSidebar()

	if !s.IsFocused() {
		t.Error("sidebar should be focused by default")
	}

	s.SetFocused(false)
	if s.IsFocused() {
		t.Error("sidebar should not be focused after SetFocused(false)")
	}

	s.SetFocused(true)
	if !s.IsFocused() {
		t.Error("sidebar should be focused after SetFocused(true)")
	}
}

func TestSidebar_SetAccounts_WithBalances(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", models.AccountTypeChecking)
	balances := map[models.ID]*service.AccountBalance{
		checking.ID: {
			AccountID:      checking.ID,
			CurrentBalance: models.MustNewMoney("1234.56"),
			ClearedBalance: models.MustNewMoney("1000.00"),
		},
	}

	s.SetAccounts([]*models.Account{checking}, balances)

	if len(s.balances) != 1 {
		t.Errorf("expected 1 balance, got %d", len(s.balances))
	}
}

func TestSidebar_Render_Empty(t *testing.T) {
	s := NewSidebar()
	styles := NewStyles()
	styles.Resize(80, 24)

	result := s.Render(styles, 20, 10)
	if result == "" {
		t.Error("Render() should not return empty string even with no items")
	}
}

func TestSidebar_Render_ZeroDimensions(t *testing.T) {
	s := NewSidebar()
	styles := NewStyles()

	if s.Render(styles, 0, 10) != "" {
		t.Error("Render() with width=0 should return empty string")
	}
	if s.Render(styles, 10, 0) != "" {
		t.Error("Render() with height=0 should return empty string")
	}
}

func TestSidebar_Render_WithAccounts(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Visa", models.AccountTypeCreditCard),
	}
	s.SetAccounts(accounts, nil)

	styles := NewStyles()
	styles.Resize(80, 24)

	result := s.Render(styles, 20, 10)
	if result == "" {
		t.Error("Render() should not return empty string with accounts")
	}
}

func TestSidebar_ClampCursor_AfterCollapse(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings]

	// Move cursor to last item (Savings)
	s.cursor = 2

	// Collapse the group - cursor should clamp to 0
	s.collapsed["Bank Accounts"] = true
	s.rebuildItems()
	s.clampCursor()

	if s.cursor != 0 {
		t.Errorf("cursor should clamp to 0 after collapse, got %d", s.cursor)
	}
}

func TestBuildGroups_EmptyAccounts(t *testing.T) {
	groups := buildGroups(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestBuildGroups_OrderAndGrouping(t *testing.T) {
	accounts := []*models.Account{
		testAccount("Brokerage", models.AccountTypeInvestment),
		testAccount("Visa", models.AccountTypeCreditCard),
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
		testAccount("Cash", models.AccountTypeCash),
	}

	groups := buildGroups(accounts)

	// Expected: Bank Accounts (Checking, Savings), Cash (Cash), Credit Cards (Visa), Investments (Brokerage)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(groups))
	}

	expectedLabels := []string{"Bank Accounts", "Cash", "Credit Cards", "Investments"}
	for i, g := range groups {
		if g.label != expectedLabels[i] {
			t.Errorf("group[%d].label = %q, want %q", i, g.label, expectedLabels[i])
		}
	}

	// Bank Accounts should have 2 accounts
	if len(groups[0].accounts) != 2 {
		t.Errorf("Bank Accounts group should have 2 accounts, got %d", len(groups[0].accounts))
	}
}

func TestBuildGroups_AllTypes(t *testing.T) {
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Savings", models.AccountTypeSavings),
		testAccount("Cash", models.AccountTypeCash),
		testAccount("Visa", models.AccountTypeCreditCard),
		testAccount("Brokerage", models.AccountTypeInvestment),
		testAccount("Mortgage", models.AccountTypeLoan),
		testAccount("House", models.AccountTypeAsset),
	}

	groups := buildGroups(accounts)

	// Bank Accounts, Cash, Credit Cards, Investments, Loans, Assets
	if len(groups) != 6 {
		t.Fatalf("expected 6 groups, got %d", len(groups))
	}

	expectedLabels := []string{"Bank Accounts", "Cash", "Credit Cards", "Investments", "Loans", "Assets"}
	for i, g := range groups {
		if g.label != expectedLabels[i] {
			t.Errorf("group[%d].label = %q, want %q", i, g.label, expectedLabels[i])
		}
	}
}

func TestSidebar_MultipleGroupCollapseExpand(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Visa", models.AccountTypeCreditCard),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Credit Cards, Visa] = 4 items

	if len(s.items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(s.items))
	}

	// Collapse first group (cursor at 0)
	s.CollapseGroup()
	// items: [Bank Accounts, Credit Cards, Visa] = 3 items
	if len(s.items) != 3 {
		t.Errorf("after collapsing first group, expected 3 items, got %d", len(s.items))
	}

	// Move to Credit Cards group and collapse it too
	s.MoveDown()
	s.CollapseGroup()
	// items: [Bank Accounts, Credit Cards] = 2 items
	if len(s.items) != 2 {
		t.Errorf("after collapsing both groups, expected 2 items, got %d", len(s.items))
	}

	// Expand first group
	s.cursor = 0
	s.ExpandGroup()
	// items: [Bank Accounts, Checking, Credit Cards] = 3 items
	if len(s.items) != 3 {
		t.Errorf("after expanding first group, expected 3 items, got %d", len(s.items))
	}
}

func TestSidebar_NavigationWithCollapseState(t *testing.T) {
	s := NewSidebar()
	accounts := []*models.Account{
		testAccount("Checking", models.AccountTypeChecking),
		testAccount("Visa", models.AccountTypeCreditCard),
	}
	s.SetAccounts(accounts, nil)

	// Collapse first group
	s.CollapseGroup()

	// Move down to Credit Cards
	s.MoveDown()
	item := s.CursorItem()
	if item == nil || item.kind != sidebarItemGroup || item.groupKey != "Credit Cards" {
		t.Error("after collapse + MoveDown, should be on Credit Cards group")
	}

	// Move down to Visa
	s.MoveDown()
	item = s.CursorItem()
	if item == nil || item.kind != sidebarItemAccount || item.account.Name != "Visa" {
		t.Error("after collapse + 2x MoveDown, should be on Visa account")
	}
}

func TestSidebar_SelectPreservesAcrossReload(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", models.AccountTypeChecking)
	accounts := []*models.Account{checking}
	s.SetAccounts(accounts, nil)

	// Select the account
	s.MoveDown()
	s.Select()

	if s.selectedAccountID != checking.ID {
		t.Fatal("account should be selected")
	}

	// Reload accounts (simulates data refresh)
	s.SetAccounts(accounts, nil)

	// Selection should persist
	if s.selectedAccountID != checking.ID {
		t.Error("selectedAccountID should persist after SetAccounts")
	}
}
