package theme

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tuitheme "github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/spf13/cobra"
)

// walCachePath returns the canonical pywal `colors.json` location. It
// honors $XDG_CACHE_HOME/wal/colors.json when the env var is set, then
// falls back to $HOME/.cache/wal/colors.json — the documented pywal
// default.
func walCachePath() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "wal", "colors.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "wal", "colors.json"), nil
}

// defaultWalThemePath returns the default destination for the generated
// theme: $XDG_CONFIG_HOME/tmoney/themes/wal.toml (or the $HOME-rooted
// equivalent). It is the same directory DiscoverUserThemes scans, so a
// successful run shows up in View → Theme without further configuration.
func defaultWalThemePath() (string, error) {
	dir, err := tuitheme.UserThemesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wal.toml"), nil
}

// newThemeGenerateFromWalCmd registers `tmoney theme generate-from-wal`.
// `--output -` streams the generated TOML to stdout; `--output PATH`
// writes to PATH; the default writes to defaultWalThemePath().
func newThemeGenerateFromWalCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "generate-from-wal",
		Short: "Generate a theme TOML file from the pywal cache",
		Long: "Reads ~/.cache/wal/colors.json (XDG_CACHE_HOME aware) and " +
			"writes a TMoney theme TOML mapping the pywal palette onto " +
			"TMoney's slot schema. By default it writes wal.toml into the " +
			"user themes directory; use --output - to print to stdout.",
		Example: "  tmoney theme generate-from-wal\n" +
			"  tmoney theme generate-from-wal --output -\n" +
			"  tmoney theme generate-from-wal --output /tmp/wal.toml",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runThemeGenerateFromWal(cmd, output, time.Now())
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"Output path for the generated theme ('-' for stdout; default: $XDG_CONFIG_HOME/tmoney/themes/wal.toml)")
	return cmd
}

func runThemeGenerateFromWal(cmd *cobra.Command, output string, ts time.Time) error {
	src, err := walCachePath()
	if err != nil {
		return err
	}
	wc, err := readWalColors(src)
	if err != nil {
		return err
	}
	tomlBody := walToThemeTOML(wc, src, ts)

	if output == "-" {
		_, err := io.WriteString(cmd.OutOrStdout(), tomlBody)
		return err
	}

	target := output
	if target == "" {
		target, err = defaultWalThemePath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create theme directory: %w", err)
	}
	if err := os.WriteFile(target, []byte(tomlBody), 0o644); err != nil {
		return fmt.Errorf("write theme file %s: %w", target, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s\n", target)
	return nil
}
