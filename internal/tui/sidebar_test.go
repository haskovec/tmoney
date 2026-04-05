package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// testAccount creates an account with the given name and type for testing.
func testAccount(name string, accountType account.Type) *account.Account {
	return &account.Account{
		BaseModel:      types.BaseModel{ID: types.NewID(), CreatedAt: types.NewTimestamp(time.Now()), UpdatedAt: types.NewTimestamp(time.Now())},
		Name:           name,
		Type:           accountType,
		Currency:       "USD",
		OpeningBalance: types.ZeroMoney,
		OpeningDate:    types.Today(),
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
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
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
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Visa", account.TypeCreditCard),
		testAccount("Brokerage", account.TypeInvestment),
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

func TestSidebar_AllGroupsAlwaysExpanded(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Visa", account.TypeCreditCard),
	}
	s.SetAccounts(accounts, nil)

	// All groups always expanded: Bank Accounts + 2 accounts + Credit Cards + 1 account = 5
	if len(s.items) != 5 {
		t.Errorf("expected 5 items (all expanded), got %d", len(s.items))
	}

	// Verify all accounts are present
	accountNames := []string{}
	for _, item := range s.items {
		if item.kind == sidebarItemAccount {
			accountNames = append(accountNames, item.account.Name)
		}
	}
	if len(accountNames) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(accountNames))
	}
}

func TestSidebar_MoveUpDown(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
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

func TestSidebar_Select_Account(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", account.TypeChecking)
	accounts := []*account.Account{checking}
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
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	s.SetAccounts(accounts, nil)

	// Cursor is on group header
	selected := s.Select()
	if selected {
		t.Error("Select() should return false for group item")
	}

	// Items should remain unchanged (no collapse)
	if len(s.items) != 2 {
		t.Errorf("expected 2 items after selecting group, got %d", len(s.items))
	}
}

func TestSidebar_SelectedAccount(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", account.TypeChecking)
	accounts := []*account.Account{checking}
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

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
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
	checking := testAccount("Checking", account.TypeChecking)
	balances := map[types.ID]*account.Balance{
		checking.ID: {
			AccountID:      checking.ID,
			CurrentBalance: types.MustNewMoney("1234.56"),
			ClearedBalance: types.MustNewMoney("1000.00"),
		},
	}

	s.SetAccounts([]*account.Account{checking}, balances)

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

func TestSidebar_Render_EmptyShowsMessage(t *testing.T) {
	s := NewSidebar()
	styles := NewStyles()
	styles.Resize(80, 24)

	result := s.Render(styles, 25, 10)
	if !strings.Contains(result, "No accounts") {
		t.Error("Render() should show 'No accounts' when empty")
	}
	if !strings.Contains(result, "Press 'n' to add") {
		t.Error("Render() should show hint to add accounts when empty")
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
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Visa", account.TypeCreditCard),
	}
	s.SetAccounts(accounts, nil)

	styles := NewStyles()
	styles.Resize(80, 24)

	result := s.Render(styles, 20, 10)
	if result == "" {
		t.Error("Render() should not return empty string with accounts")
	}
}

func TestSidebar_SetCursor(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings] = 3 items

	s.SetCursor(1)
	if s.cursor != 1 {
		t.Errorf("SetCursor(1): cursor = %d, want 1", s.cursor)
	}

	s.SetCursor(2)
	if s.cursor != 2 {
		t.Errorf("SetCursor(2): cursor = %d, want 2", s.cursor)
	}

	// Clamps to valid range
	s.SetCursor(10)
	if s.cursor != 2 {
		t.Errorf("SetCursor(10): cursor = %d, want 2 (clamped)", s.cursor)
	}

	s.SetCursor(-1)
	if s.cursor != 0 {
		t.Errorf("SetCursor(-1): cursor = %d, want 0 (clamped)", s.cursor)
	}
}

func TestSidebar_SetCursor_Empty(t *testing.T) {
	s := NewSidebar()

	s.SetCursor(5)
	if s.cursor != 0 {
		t.Errorf("SetCursor on empty sidebar: cursor = %d, want 0", s.cursor)
	}
}

func TestSidebar_ItemCount(t *testing.T) {
	s := NewSidebar()

	if s.ItemCount() != 0 {
		t.Errorf("ItemCount on empty sidebar = %d, want 0", s.ItemCount())
	}

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Visa", account.TypeCreditCard),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Credit Cards, Visa] = 4 items

	if s.ItemCount() != 4 {
		t.Errorf("ItemCount = %d, want 4", s.ItemCount())
	}
}

func TestSidebar_HitTest(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings] = 3 items

	tests := []struct {
		name string
		y    int
		want int
	}{
		{"first item (group header)", 0, 0},
		{"second item (Checking)", 1, 1},
		{"third item (Savings)", 2, 2},
		{"out of range", 3, -1},
		{"negative", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.HitTest(tt.y)
			if got != tt.want {
				t.Errorf("HitTest(%d) = %d, want %d", tt.y, got, tt.want)
			}
		})
	}
}

func TestSidebar_HitTest_Empty(t *testing.T) {
	s := NewSidebar()

	if got := s.HitTest(0); got != -1 {
		t.Errorf("HitTest(0) on empty sidebar = %d, want -1", got)
	}
}

func TestSidebar_HitTest_WithScroll(t *testing.T) {
	s := NewSidebar()
	// Create enough accounts to require scrolling
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Visa", account.TypeCreditCard),
		testAccount("MC", account.TypeCreditCard),
		testAccount("Brokerage", account.TypeInvestment),
	}
	s.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings, Credit Cards, Visa, MC, Investments, Brokerage] = 8

	s.scrollOffset = 3 // Start from "Credit Cards"

	// y=0 should map to item 3 (Credit Cards)
	if got := s.HitTest(0); got != 3 {
		t.Errorf("HitTest(0) with scrollOffset=3 = %d, want 3", got)
	}

	// y=2 should map to item 5 (MC)
	if got := s.HitTest(2); got != 5 {
		t.Errorf("HitTest(2) with scrollOffset=3 = %d, want 5", got)
	}
}

func TestSidebar_ScrollOnMoveDown(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Visa", account.TypeCreditCard),
		testAccount("MC", account.TypeCreditCard),
		testAccount("Brokerage", account.TypeInvestment),
	}
	s.SetAccounts(accounts, nil)
	// 8 items total

	styles := NewStyles()
	styles.Resize(80, 24)

	// Move cursor to the bottom and render with small viewport
	for range 7 {
		s.MoveDown()
	}
	if s.cursor != 7 {
		t.Fatalf("cursor = %d, want 7", s.cursor)
	}

	// Render with viewport of 5 lines
	s.Render(styles, 20, 5)

	// scrollOffset should have adjusted so cursor is visible
	if s.scrollOffset < 3 {
		t.Errorf("scrollOffset = %d, should be >= 3 to show cursor at 7 in viewport of 5", s.scrollOffset)
	}
}

func TestSidebar_ScrollClamp(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	s.SetAccounts(accounts, nil)
	// 2 items: [Bank Accounts, Checking]

	// Force scroll offset beyond items
	s.scrollOffset = 10
	s.clampScroll(5)

	if s.scrollOffset != 0 {
		t.Errorf("scrollOffset should clamp to 0 when all items fit, got %d", s.scrollOffset)
	}
}

func TestSidebar_ScrollFollowsCursor(t *testing.T) {
	s := NewSidebar()
	accounts := []*account.Account{
		testAccount("A", account.TypeChecking),
		testAccount("B", account.TypeSavings),
		testAccount("C", account.TypeCreditCard),
		testAccount("D", account.TypeInvestment),
		testAccount("E", account.TypeLoan),
	}
	s.SetAccounts(accounts, nil)
	// 4 groups + 5 accounts = 9 items (Checking/Savings share Bank Accounts group)

	// Set cursor near end
	s.cursor = 8
	s.clampScroll(4)

	// Cursor should be visible
	if s.cursor < s.scrollOffset || s.cursor >= s.scrollOffset+4 {
		t.Errorf("cursor %d not visible with scrollOffset=%d viewport=4", s.cursor, s.scrollOffset)
	}

	// Now move cursor to top
	s.cursor = 0
	s.clampScroll(4)

	if s.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 when cursor is at top, got %d", s.scrollOffset)
	}
}

func TestSidebar_SelectPreservesAcrossReload(t *testing.T) {
	s := NewSidebar()
	checking := testAccount("Checking", account.TypeChecking)
	accounts := []*account.Account{checking}
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

func TestBuildGroups_EmptyAccounts(t *testing.T) {
	groups := buildGroups(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestBuildGroups_OrderAndGrouping(t *testing.T) {
	accounts := []*account.Account{
		testAccount("Brokerage", account.TypeInvestment),
		testAccount("Visa", account.TypeCreditCard),
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Cash", account.TypeCash),
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
	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Cash", account.TypeCash),
		testAccount("Visa", account.TypeCreditCard),
		testAccount("Brokerage", account.TypeInvestment),
		testAccount("Mortgage", account.TypeLoan),
		testAccount("House", account.TypeAsset),
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
