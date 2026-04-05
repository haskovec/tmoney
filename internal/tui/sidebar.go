package tui

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// accountGroupOrder defines the display order of account type groups in the sidebar.
var accountGroupOrder = []account.Type{
	account.TypeChecking,
	account.TypeSavings,
	account.TypeCash,
	account.TypeCreditCard,
	account.TypeInvestment,
	account.TypeLoan,
	account.TypeAsset,
}

// accountGroupLabels maps account types to their sidebar group display names.
var accountGroupLabels = map[account.Type]string{
	account.TypeChecking:   "Bank Accounts",
	account.TypeSavings:    "Bank Accounts",
	account.TypeCash:       "Cash",
	account.TypeCreditCard: "Credit Cards",
	account.TypeInvestment: "Investments",
	account.TypeLoan:       "Loans",
	account.TypeAsset:      "Assets",
}

// sidebarItemKind distinguishes between group headers and account items.
type sidebarItemKind int

const (
	sidebarItemGroup sidebarItemKind = iota
	sidebarItemAccount
)

// sidebarItem represents a single navigable item in the sidebar.
type sidebarItem struct {
	kind      sidebarItemKind
	groupKey  string           // group label
	account   *account.Account // non-nil for account items
	accountID types.ID         // shortcut for account items
}

// accountGroup holds a named group of accounts for sidebar display.
type accountGroup struct {
	label    string
	accounts []*account.Account
}

// Sidebar manages the account sidebar state and rendering.
type Sidebar struct {
	// All visible items (groups + accounts) in display order
	items []sidebarItem

	// Navigation state
	cursor       int
	scrollOffset int

	// Selected account (persists across navigation)
	selectedAccountID types.ID

	// Cached data
	accounts []*account.Account
	balances map[types.ID]*account.Balance

	// Focus state
	focused bool
}

// NewSidebar creates a new Sidebar with default state.
func NewSidebar() *Sidebar {
	return &Sidebar{
		balances: make(map[types.ID]*account.Balance),
		focused:  true,
	}
}

// SetAccounts updates the sidebar with a new account list and balances.
func (s *Sidebar) SetAccounts(accounts []*account.Account, balances map[types.ID]*account.Balance) {
	s.accounts = accounts
	if balances != nil {
		s.balances = balances
	}
	s.rebuildItems()
}

// SetFocused sets whether the sidebar has input focus.
func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

// IsFocused returns whether the sidebar has input focus.
func (s *Sidebar) IsFocused() bool {
	return s.focused
}

// SelectedAccountID returns the currently selected account ID.
func (s *Sidebar) SelectedAccountID() types.ID {
	return s.selectedAccountID
}

// SelectedAccount returns the currently selected account, or nil.
func (s *Sidebar) SelectedAccount() *account.Account {
	if s.selectedAccountID.IsNil() {
		return nil
	}
	for _, a := range s.accounts {
		if a.ID == s.selectedAccountID {
			return a
		}
	}
	return nil
}

// CursorItem returns the item at the current cursor position, or nil.
func (s *Sidebar) CursorItem() *sidebarItem {
	if s.cursor < 0 || s.cursor >= len(s.items) {
		return nil
	}
	return &s.items[s.cursor]
}

// MoveUp moves the cursor up one position.
func (s *Sidebar) MoveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

// MoveDown moves the cursor down one position.
func (s *Sidebar) MoveDown() {
	if s.cursor < len(s.items)-1 {
		s.cursor++
	}
}

// Select selects the item at the cursor. Returns true if an account was selected.
func (s *Sidebar) Select() bool {
	item := s.CursorItem()
	if item == nil {
		return false
	}

	if item.kind == sidebarItemAccount {
		s.selectedAccountID = item.accountID
		return true
	}
	return false
}

// buildGroups organizes accounts into display groups.
func buildGroups(accounts []*account.Account) []accountGroup {
	// Group accounts by their sidebar group label
	byGroup := make(map[string][]*account.Account)
	for _, a := range accounts {
		label := accountGroupLabels[a.Type]
		if label == "" {
			label = a.Type.DisplayName()
		}
		byGroup[label] = append(byGroup[label], a)
	}

	// Build ordered groups (deduplicate labels since checking/savings share "Bank Accounts")
	var groups []accountGroup
	seen := make(map[string]bool)
	for _, at := range accountGroupOrder {
		label := accountGroupLabels[at]
		if label == "" {
			label = at.DisplayName()
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		accts := byGroup[label]
		if len(accts) > 0 {
			groups = append(groups, accountGroup{
				label:    label,
				accounts: accts,
			})
		}
	}
	return groups
}

// rebuildItems reconstructs the flat item list from accounts.
// All groups are always expanded — no collapse support.
func (s *Sidebar) rebuildItems() {
	groups := buildGroups(s.accounts)

	var items []sidebarItem
	for _, g := range groups {
		items = append(items, sidebarItem{
			kind:     sidebarItemGroup,
			groupKey: g.label,
		})

		for _, a := range g.accounts {
			items = append(items, sidebarItem{
				kind:      sidebarItemAccount,
				groupKey:  g.label,
				account:   a,
				accountID: a.ID,
			})
		}
	}
	s.items = items
	s.clampCursor()
}

// clampCursor ensures the cursor is within valid bounds.
func (s *Sidebar) clampCursor() {
	if len(s.items) == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= len(s.items) {
		s.cursor = len(s.items) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// clampScroll adjusts scrollOffset so the cursor is visible within the viewport.
func (s *Sidebar) clampScroll(viewportHeight int) {
	if viewportHeight <= 0 || len(s.items) == 0 {
		s.scrollOffset = 0
		return
	}

	// Ensure cursor is visible
	if s.cursor < s.scrollOffset {
		s.scrollOffset = s.cursor
	}
	if s.cursor >= s.scrollOffset+viewportHeight {
		s.scrollOffset = s.cursor - viewportHeight + 1
	}

	// Clamp scroll offset
	maxOffset := max(len(s.items)-viewportHeight, 0)
	if s.scrollOffset > maxOffset {
		s.scrollOffset = maxOffset
	}
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}
}

// SetCursor sets the cursor to the given position, clamping to valid bounds.
func (s *Sidebar) SetCursor(pos int) {
	s.cursor = pos
	s.clampCursor()
}

// ItemCount returns the number of visible items.
func (s *Sidebar) ItemCount() int {
	return len(s.items)
}

// HitTest determines which sidebar item was clicked at row y.
// y is relative to the top of the sidebar content (0-based).
// Accounts for the current scroll offset.
// Returns the item index, or -1 if out of range.
func (s *Sidebar) HitTest(y int) int {
	idx := s.scrollOffset + y
	if idx >= 0 && idx < len(s.items) {
		return idx
	}
	return -1
}

// Render renders the sidebar content for the given dimensions.
func (s *Sidebar) Render(styles Styles, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	s.clampScroll(height)

	var lines []string

	if len(s.items) == 0 {
		lines = append(lines, "")
		lines = append(lines, styles.Muted.Render("  No accounts"))
		lines = append(lines, styles.Muted.Render("  Press 'n' to add"))
	}

	// Render only the visible window of items
	end := min(s.scrollOffset+height, len(s.items))
	for i := s.scrollOffset; i < end; i++ {
		line := s.renderItem(styles, s.items[i], i, width)
		lines = append(lines, line)
	}

	// Pad with empty lines if needed
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	// Truncate if too many lines
	if len(lines) > height {
		lines = lines[:height]
	}

	content := strings.Join(lines, "\n")

	return styles.Sidebar.
		Width(width).
		Height(height).
		Render(content)
}

// renderItem renders a single sidebar item.
func (s *Sidebar) renderItem(styles Styles, item sidebarItem, index int, width int) string {
	isCursor := s.focused && index == s.cursor
	isSelected := item.kind == sidebarItemAccount && item.accountID == s.selectedAccountID

	switch item.kind {
	case sidebarItemGroup:
		return s.renderGroupHeader(styles, item, isCursor, width)
	case sidebarItemAccount:
		return s.renderAccountItem(styles, item, isCursor, isSelected, width)
	}
	return ""
}

// renderGroupHeader renders a group header line.
func (s *Sidebar) renderGroupHeader(styles Styles, item sidebarItem, isCursor bool, width int) string {
	text := fmt.Sprintf(" %s", item.groupKey)

	// Truncate if needed
	if len(text) > width-1 {
		text = text[:width-1]
	}

	// Pad to width
	if len(text) < width {
		text = text + strings.Repeat(" ", width-len(text))
	}

	if isCursor {
		return styles.SelectedRow.Render(text)
	}
	return styles.SidebarGroup.Render(text)
}

// renderAccountItem renders an account line.
func (s *Sidebar) renderAccountItem(styles Styles, item sidebarItem, isCursor, isSelected bool, width int) string {
	indicator := " "
	if isSelected {
		indicator = "◀"
	}

	name := item.account.Name
	// Leave room for "  ▸ " prefix and " ◀" suffix
	maxNameLen := max(width-6, 3)
	if len(name) > maxNameLen {
		name = name[:maxNameLen-1] + "…"
	}

	text := fmt.Sprintf("  ▸ %s %s", name, indicator)

	// Pad to width
	if len(text) < width {
		text = text + strings.Repeat(" ", width-len(text))
	}

	if isCursor {
		return styles.SelectedRow.Render(text)
	}
	return styles.SidebarItem.Render(text)
}
