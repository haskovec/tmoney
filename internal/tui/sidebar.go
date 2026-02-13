package tui

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// accountGroupOrder defines the display order of account type groups in the sidebar.
var accountGroupOrder = []models.AccountType{
	models.AccountTypeChecking,
	models.AccountTypeSavings,
	models.AccountTypeCash,
	models.AccountTypeCreditCard,
	models.AccountTypeInvestment,
	models.AccountTypeLoan,
	models.AccountTypeAsset,
}

// accountGroupLabels maps account types to their sidebar group display names.
var accountGroupLabels = map[models.AccountType]string{
	models.AccountTypeChecking:   "Bank Accounts",
	models.AccountTypeSavings:    "Bank Accounts",
	models.AccountTypeCash:       "Cash",
	models.AccountTypeCreditCard: "Credit Cards",
	models.AccountTypeInvestment: "Investments",
	models.AccountTypeLoan:       "Loans",
	models.AccountTypeAsset:      "Assets",
}

// sidebarItemKind distinguishes between group headers and account items.
type sidebarItemKind int

const (
	sidebarItemGroup   sidebarItemKind = iota
	sidebarItemAccount
)

// sidebarItem represents a single navigable item in the sidebar.
type sidebarItem struct {
	kind      sidebarItemKind
	groupKey  string           // group label (used as key for collapse state)
	account   *models.Account  // non-nil for account items
	accountID models.ID        // shortcut for account items
}

// accountGroup holds a named group of accounts for sidebar display.
type accountGroup struct {
	label    string
	accounts []*models.Account
}

// Sidebar manages the account sidebar state and rendering.
type Sidebar struct {
	// All visible items (groups + accounts) in display order
	items []sidebarItem

	// Navigation state
	cursor int

	// Collapse state: group label -> collapsed
	collapsed map[string]bool

	// Selected account (persists across navigation)
	selectedAccountID models.ID

	// Cached data
	accounts []*models.Account
	balances map[models.ID]*service.AccountBalance

	// Focus state
	focused bool
}

// NewSidebar creates a new Sidebar with default state.
func NewSidebar() *Sidebar {
	return &Sidebar{
		collapsed: make(map[string]bool),
		balances:  make(map[models.ID]*service.AccountBalance),
		focused:   true,
	}
}

// SetAccounts updates the sidebar with a new account list and balances.
func (s *Sidebar) SetAccounts(accounts []*models.Account, balances map[models.ID]*service.AccountBalance) {
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
func (s *Sidebar) SelectedAccountID() models.ID {
	return s.selectedAccountID
}

// SelectedAccount returns the currently selected account, or nil.
func (s *Sidebar) SelectedAccount() *models.Account {
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

// ToggleCollapse toggles the collapse state of the group at the cursor.
func (s *Sidebar) ToggleCollapse() {
	item := s.CursorItem()
	if item == nil {
		return
	}

	groupKey := item.groupKey
	if item.kind == sidebarItemAccount {
		// Use the group key of the account's parent group
		groupKey = item.groupKey
	}

	s.collapsed[groupKey] = !s.collapsed[groupKey]
	s.rebuildItems()
	s.clampCursor()
}

// CollapseGroup collapses the group at the cursor (left arrow).
func (s *Sidebar) CollapseGroup() {
	item := s.CursorItem()
	if item == nil {
		return
	}

	groupKey := item.groupKey

	if item.kind == sidebarItemAccount {
		// If on an account, move cursor to the group header
		for i, it := range s.items {
			if it.kind == sidebarItemGroup && it.groupKey == groupKey {
				s.cursor = i
				break
			}
		}
		return
	}

	// If on a group header, collapse it
	if !s.collapsed[groupKey] {
		s.collapsed[groupKey] = true
		s.rebuildItems()
		s.clampCursor()
	}
}

// ExpandGroup expands the group at the cursor (right arrow).
func (s *Sidebar) ExpandGroup() {
	item := s.CursorItem()
	if item == nil {
		return
	}

	groupKey := item.groupKey

	if item.kind == sidebarItemGroup && s.collapsed[groupKey] {
		s.collapsed[groupKey] = false
		s.rebuildItems()
		s.clampCursor()
	}
}

// Select selects the item at the cursor. Returns true if an account was selected.
func (s *Sidebar) Select() bool {
	item := s.CursorItem()
	if item == nil {
		return false
	}

	switch item.kind {
	case sidebarItemGroup:
		s.ToggleCollapse()
		return false
	case sidebarItemAccount:
		s.selectedAccountID = item.accountID
		return true
	}
	return false
}

// buildGroups organizes accounts into display groups.
func buildGroups(accounts []*models.Account) []accountGroup {
	// Group accounts by their sidebar group label
	byGroup := make(map[string][]*models.Account)
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

// rebuildItems reconstructs the flat item list from accounts and collapse state.
func (s *Sidebar) rebuildItems() {
	groups := buildGroups(s.accounts)

	var items []sidebarItem
	for _, g := range groups {
		items = append(items, sidebarItem{
			kind:     sidebarItemGroup,
			groupKey: g.label,
		})

		if !s.collapsed[g.label] {
			for _, a := range g.accounts {
				items = append(items, sidebarItem{
					kind:      sidebarItemAccount,
					groupKey:  g.label,
					account:   a,
					accountID: a.ID,
				})
			}
		}
	}
	s.items = items
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

// Render renders the sidebar content for the given dimensions.
func (s *Sidebar) Render(styles Styles, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	var lines []string

	if len(s.items) == 0 {
		lines = append(lines, "")
		lines = append(lines, styles.Muted.Render("  No accounts"))
		lines = append(lines, styles.Muted.Render("  Press 'n' to add"))
	}

	for i, item := range s.items {
		line := s.renderItem(styles, item, i, width)
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
	arrow := "▼"
	if s.collapsed[item.groupKey] {
		arrow = "▶"
	}

	text := fmt.Sprintf("%s %s", arrow, item.groupKey)

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
