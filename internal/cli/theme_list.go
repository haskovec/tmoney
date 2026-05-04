package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/spf13/cobra"
)

// activeThemeIDFn returns the active theme ID. Wrapped as a var so
// tests can stub the config lookup. A failed config load is treated
// as "no active theme" — listing should never fail because of a
// transient config-read error.
var activeThemeIDFn = func() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	return cfg.Theme
}

// newThemeListCmd registers `tmoney theme list`. It prints a
// tabwriter-aligned table with columns ID, SOURCE, NAME, ACTIVE.
// Built-ins compiled into the binary appear with source "built-in";
// themes from $XDG_CONFIG_HOME/tmoney/themes/ with source "user". A
// user theme that shares an ID with a built-in is reported once with
// source "user" since LoadTheme would prefer it. The currently
// configured theme is marked with "*" in the ACTIVE column.
func newThemeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available themes (built-in and user)",
		Long: "List every theme TMoney can load — both built-ins compiled " +
			"into the binary and user-authored themes from " +
			"$XDG_CONFIG_HOME/tmoney/themes/. The currently active theme " +
			"(read from config) is marked with '*' in the ACTIVE column.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runThemeList(cmd.OutOrStdout(), activeThemeIDFn())
		},
	}
}

// themeRow is one row in the listing table.
type themeRow struct {
	ID     string
	Source string
	Name   string
	Active bool
}

// listThemeRows merges built-ins and user themes into a sorted slice.
// User themes shadow built-ins by ID; the shadowed entry reports
// source "user" (matching LoadTheme's resolution order) and uses the
// user theme's display name.
func listThemeRows(activeID string) ([]themeRow, error) {
	userIDs, err := theme.DiscoverUserThemes()
	if err != nil {
		return nil, err
	}
	userSet := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		userSet[id] = struct{}{}
	}

	rows := make([]themeRow, 0, len(theme.BuiltinIDs())+len(userIDs))
	seen := make(map[string]bool)
	for _, id := range theme.BuiltinIDs() {
		source := "built-in"
		if _, ok := userSet[id]; ok {
			source = "user"
		}
		rows = append(rows, themeRow{
			ID:     id,
			Source: source,
			Name:   themeDisplayName(id),
			Active: id == activeID,
		})
		seen[id] = true
	}
	for _, id := range userIDs {
		if seen[id] {
			continue
		}
		rows = append(rows, themeRow{
			ID:     id,
			Source: "user",
			Name:   themeDisplayName(id),
			Active: id == activeID,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// themeDisplayName returns the human-readable Name for a theme ID,
// preferring a user override over the built-in. A theme that fails
// to parse drops to its ID stem so the listing still renders.
func themeDisplayName(id string) string {
	t, _, err := theme.LoadTheme(id)
	if err != nil || t == nil || t.Name == "" {
		return id
	}
	return t.Name
}

func runThemeList(out io.Writer, activeID string) error {
	rows, err := listThemeRows(activeID)
	if err != nil {
		return fmt.Errorf("discover themes: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tSOURCE\tNAME\tACTIVE"); err != nil {
		return err
	}
	for _, r := range rows {
		marker := ""
		if r.Active {
			marker = "*"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ID, r.Source, r.Name, marker); err != nil {
			return err
		}
	}
	return w.Flush()
}
