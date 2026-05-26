package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// themeReloadFailedMsg is sent when reloadTheme cannot load or apply
// the requested theme — the active palette and cfg are left untouched.
// reloadTheme also surfaces the failure synchronously (toast + log) so
// downstream consumers can ignore this message; it stays for tests and
// any future handler that wants to react explicitly.
type themeReloadFailedMsg struct {
	id  string
	err error
}

// reloadTheme loads the theme with the given ID, applies it to
// a.styles, persists the ID into a.cfg, and returns a tea.Cmd that
// emits a tea.WindowSizeMsg matching the App's current dimensions so
// the next render reflects the new palette.
//
// LoadTheme (TH-026) consults the user theme directory first and
// falls back to the embedded built-ins, so user overrides take effect
// without any extra wiring here. On failure (unknown ID, parse error)
// the styles, palette, and config are left unchanged and the returned
// cmd emits a themeReloadFailedMsg.
//
// TH-032: parse issues encountered during a successful load and any
// failure-path error are appended to the applog and surfaced as a
// status-bar toast describing the issue count. The returned cmd is
// batched with a widget.ClearToastCmd so the toast clears after widget.ToastDuration.
// Successful loads with zero issues set no toast and return the bare
// WindowSizeMsg cmd.
func (a *App) reloadTheme(id string) tea.Cmd {
	t, issues, err := theme.LoadTheme(id)
	if err != nil {
		a.surfaceThemeFailure(id, err)
		return tea.Batch(
			func() tea.Msg { return themeReloadFailedMsg{id: id, err: err} },
			widget.ClearToastCmd(),
		)
	}

	a.styles.ApplyTheme(t)
	a.styles.Resize(a.width, a.height)

	if a.cfg != nil {
		a.cfg.Theme = id
		// Save is best-effort: under `go test` it's a no-op, and
		// in production a write failure is non-fatal — the theme is
		// already live in memory.
		_ = a.cfg.Save()
	}

	width, height := a.width, a.height
	sizeCmd := func() tea.Msg {
		return tea.WindowSizeMsg{Width: width, Height: height}
	}

	if len(issues) == 0 {
		return sizeCmd
	}
	a.surfaceThemeIssues(id, issues)
	return tea.Batch(sizeCmd, widget.ClearToastCmd())
}

// surfaceThemeIssues appends each parse issue to the applog file and
// sets a status-bar toast summarizing the count. Format mirrors the
// spec: "Theme '<id>': <N> issues, see <log path>".
func (a *App) surfaceThemeIssues(id string, issues []theme.Issue) {
	for _, iss := range issues {
		_ = applog.Append("theme", formatThemeIssue(id, iss))
	}
	if a.statusbar != nil {
		a.statusbar.SetToast(formatThemeToast(id, len(issues)), widget.NotificationAlert)
	}
}

// surfaceThemeFailure logs an unparseable / missing-theme failure and
// sets a toast pointing the user at the log file.
func (a *App) surfaceThemeFailure(id string, err error) {
	_ = applog.Append("theme", fmt.Sprintf("%s: failed to load: %v", id, err))
	if a.statusbar != nil {
		text := fmt.Sprintf("Theme %q: failed to load", id)
		if path, perr := applog.LogPath(); perr == nil {
			text = fmt.Sprintf("%s, see %s", text, path)
		}
		a.statusbar.SetToast(text, widget.NotificationAlert)
	}
}

// formatThemeIssue renders one parse issue as a single log line.
// Includes the theme ID, slot kind, key, the offending raw value (if
// any), and the parser's reason text.
func formatThemeIssue(id string, iss theme.Issue) string {
	if iss.Value != "" {
		return fmt.Sprintf("%s: %s %s=%q (%s)", id, iss.Kind, iss.Key, iss.Value, iss.Reason)
	}
	return fmt.Sprintf("%s: %s %s (%s)", id, iss.Kind, iss.Key, iss.Reason)
}

// formatThemeToast renders the user-facing toast text for a theme load
// that produced N parse issues. Includes the log path so users know
// where to look for details.
func formatThemeToast(id string, n int) string {
	noun := "issues"
	if n == 1 {
		noun = "issue"
	}
	base := fmt.Sprintf("Theme %q: %d %s", id, n, noun)
	if path, err := applog.LogPath(); err == nil {
		return fmt.Sprintf("%s, see %s", base, path)
	}
	return base
}
