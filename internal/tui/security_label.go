package tui

import "github.com/haskovec/tmoney/internal/security"

// securityLabel returns the identifier shown for a security in narrow table
// cells and pickers: the ticker when present, otherwise the (full) name. A
// tickerless security — e.g. a collective trust carried by name + ISIN — would
// otherwise render as a blank cell. Table cells truncate with an ellipsis, so a
// long name is clipped to the column width.
func securityLabel(sec *security.Security) string {
	if sec.Ticker != "" {
		return sec.Ticker
	}
	return sec.Name
}
