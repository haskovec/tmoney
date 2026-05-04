# Implementation Plan: Theme System & CLI Scaffold

This document defines the order in which the theme system (specs/theming.md) and the CLI router scaffold required to support it (specs/cli-router.md) should be implemented. Each item represents one small session of work following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Specs:
- `specs/theming.md` — theme system feature spec
- `specs/cli-router.md` — CLI router design (this plan only covers the scaffold + `theme` + `version` batch; further migrations are out of scope here)

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered to:

1. Establish the CLI scaffold (Phase 1) so subsequent theme subcommands have somewhere to live.
2. Build the theme data model and parser (Phase 2) before wiring it into the running app — pure logic, easy to TDD.
3. Add the embedded built-in themes (Phase 3) so we have content for the loader to work with.
4. Refactor inline `lipgloss.NewStyle()` color call sites into `Styles` fields (Phase 4) so live-swap propagates correctly. Done before live-swap is wired.
5. Implement the live-swap mechanism (Phase 5).
6. Wire the View → Theme menu (Phase 6) — first user-visible theme switching.
7. Add user theme directory discovery (Phase 7).
8. Persist active theme in config (Phase 8).
9. Surface theme-load errors via status-bar toast + log file (Phase 9).
10. Build the pywal helper subcommand (Phase 10).
11. Add `tmoney theme list` (Phase 11).
12. Documentation and README updates (Phase 12).

Phases 5 and 6 could be merged — live-swap and the menu trigger are tightly coupled — but the plan separates them so the live-swap mechanism can be exercised by unit tests via a programmatic `reloadTheme()` call before menu UI is involved.

Phases 9 (error surfacing) is intentionally late: the system works without it, but it's necessary before declaring the feature shippable. It depends on Phases 2 and 5 being in place.

---

## Phase 1: CLI Scaffold

This phase introduces the Cobra-based command router and moves `main.go` to the module root. **No legacy flag handling changes** — the existing `parseArgs`/`run()` flow is preserved verbatim; the Cobra root command's no-subcommand path delegates to it. After this phase the build is reorganized but behavior is identical except that `tmoney version` and `tmoney --help` now produce Cobra-generated output.

- [x] **TH-001 — Add Cobra dependency**
  - GREEN: `go get github.com/spf13/cobra@latest`, then `go mod tidy`. Confirm `go build ./...` is still clean.
  - No tests; pure dependency addition.

- [x] **TH-002 — Create `internal/cli/` package and move existing files**
  - GREEN: create `internal/cli/`. Move (with `git mv` to preserve history) `cmd/tmoney/args.go`, `cmd/tmoney/args_test.go`, `cmd/tmoney/commands.go`, `cmd/tmoney/commands_test.go`, `cmd/tmoney/format.go`, `cmd/tmoney/format_test.go`, `cmd/tmoney/update_prices.go`, `cmd/tmoney/update_prices_test.go`, `cmd/tmoney/main_test.go` to `internal/cli/`.
  - Update package declaration from `package main` to `package cli` in each file.
  - Move `cmd/tmoney/main.go` content (the body of `run()`) into a new `internal/cli/legacy.go` exposing a `RunLegacy(args []string, stdout, stderr io.Writer) error` function. The version vars (`Version`, `BuildTime`, `GitCommit`) move to `internal/cli/version.go`.
  - Delete `cmd/tmoney/some-file.tdb` (test artifact) and add `*.tdb` to `.gitignore` if not already present.
  - Confirm `go build ./internal/cli/...` clean. Tests run via `go test ./internal/cli/...`.

- [x] **TH-003 — Add `main.go` at module root**
  - RED: not applicable.
  - GREEN: write `/main.go`:
    ```go
    package main

    import (
        "os"
        "github.com/haskovec/tmoney/internal/cli"
    )

    func main() {
        if err := cli.Execute(); err != nil {
            os.Exit(1)
        }
    }
    ```
  - Update `Makefile`: change `go build -o tmoney ./cmd/tmoney` to `go build -o tmoney .`.
  - Delete the now-empty `cmd/tmoney/` directory.
  - Confirm `make build` produces a working binary.

- [x] **TH-004 — Cobra root command with no-args fallback**
  - RED: tests in `internal/cli/root_test.go` for `Execute()`:
    - `tmoney` (no args) → calls TUI launcher (mock the launcher and assert it was invoked).
    - `tmoney foo.tdb` → calls TUI launcher with file set to `foo.tdb`.
    - `tmoney --file=foo.tdb` → same.
    - Unknown legacy flag like `--list-accounts` → falls through to `RunLegacy` (mock and assert).
    - `tmoney --help` → exits 0, output contains "Usage:".
  - GREEN: implement `internal/cli/root.go` with a Cobra root command. Persistent flag `--file/-f`. The root's `RunE` checks: if `args` looks like a legacy flag invocation (starts with `--`) call `RunLegacy(os.Args[1:], os.Stdout, os.Stderr)`; else launch TUI with the resolved file path (positional arg or `--file` flag, falling back to config). `Execute()` returns the Cobra command's error.

- [x] **TH-005 — `tmoney version` subcommand**
  - RED: test in `internal/cli/version_test.go` that `tmoney version` prints the `Version`, `BuildTime`, `GitCommit` values.
  - GREEN: implement `internal/cli/version.go` registering a `version` subcommand on the root. Format: `tmoney <version>` plus build/commit metadata on subsequent lines.

- [x] **TH-006 — `tmoney --help` smoke test**
  - RED: test asserts `tmoney --help` output contains the strings `version`, `Available Commands`, and `Usage:`.
  - GREEN: ensure root command has a `Short` description and at least one example.

## Phase 2: Theme Data Model & Parser

Pure logic, no UI. Builds the schema definition, TOML parser, and validation/fallback machinery.

- [ ] **TH-007 — `internal/tui/theme/theme.go` package skeleton**
  - GREEN: create new package `internal/tui/theme`. Define the `Theme` struct with all 27 color slots, `BorderStyle` enum (`single`/`double`/`rounded`/`thick`), symbol fields, and shortcut display options. Include a `defaultTheme()` constructor returning a `Theme` populated with the existing `Color*` values from `internal/tui/styles.go` (this is the fallback used when slots are missing).
  - Test: `TestDefaultTheme_AllSlotsPopulated` — every field on the struct is non-zero (or the documented sentinel for transparent backgrounds).

- [ ] **TH-008 — TOML parser: valid input**
  - RED: test `TestParse_ValidTheme` parses a fixture `testdata/turbo-vision-min.toml` (full Turbo Vision theme as documented in the spec) and asserts every slot has the expected value.
  - GREEN: add `github.com/BurntSushi/toml` dependency. Implement `Parse(data []byte) (*Theme, []Issue, error)` in `theme/parser.go`. Returns the parsed theme, a list of non-fatal `Issue`s, and an error (only for unparseable TOML).

- [ ] **TH-009 — TOML parser: malformed values fall back per-slot**
  - RED: test `TestParse_MalformedHex` with a theme containing `text.negative = "not-a-color"` — assert the parsed theme has `text.negative` equal to the default theme's value, and one `Issue` is reported with the slot name.
  - Test `TestParse_OutOfRange256` with `text.muted = "999"` — same expectation.
  - Test `TestParse_BadBorderStyle` with `border_style = "wavy"` — falls back to `single`, issue reported.
  - GREEN: implement value validation. Hex regex `^#[0-9a-fA-F]{6}$`. ANSI 256: integer 0-255 from string. `border_style` must be one of the four enum values. On any parse failure for a slot, copy the default theme's value and append an `Issue`.

- [ ] **TH-010 — TOML parser: unknown keys logged, ignored**
  - RED: test `TestParse_UnknownKey` with a theme containing `windows.bg = "#000"` (typo). Assert the theme still loads and an `Issue` of kind `IssueUnknownKey` lists the offending key.
  - GREEN: BurntSushi/toml's `MetaData.Undecoded()` lists keys that didn't match any struct field; iterate it and emit issues.

- [ ] **TH-011 — TOML parser: missing slots fall back to default**
  - RED: test `TestParse_MinimalTheme` with `name = "Just Red"\ntext.negative = "#ff0000"` — assert all slots except `text.negative` and `name` equal the default theme's values.
  - GREEN: parser starts from `defaultTheme()` and selectively overwrites slots that the TOML provides.

- [ ] **TH-012 — TOML parser: empty `*.bg` slots preserved as transparent**
  - RED: test `TestParse_EmptyBackground` with `window.bg = ""` — assert the parsed theme's `Window.Bg` is the sentinel value meaning "transparent" (e.g., `lipgloss.Color("")` or a typed nullable). Distinct from "key not present" which means "fall back to default."
  - GREEN: model background slots as `*string` or use a sentinel.

- [ ] **TH-013 — TOML parser: name fallback to filename stem**
  - RED: test `TestParseFromFile_NoName` parses a theme file with no `name` field and asserts the returned theme's `Name` is the filename stem.
  - GREEN: add `ParseFromFile(path string) (*Theme, []Issue, error)` that reads the file and falls back to the stem if `name` is empty.

## Phase 3: Embedded Built-in Themes

Author the three TOML files and embed them.

- [ ] **TH-014 — Author `default.toml`**
  - GREEN: create `internal/tui/themes/default.toml` exactly reproducing today's palette from `styles.go`. Background slots empty for transparency. `border_style = "single"`. `*.shortcut.underline = true`, `*.shortcut.fg` unset.
  - Test: `TestEmbeddedDefault_MatchesCurrentPalette` parses the embedded file and asserts every `Color*` from `styles.go` matches the corresponding slot.

- [ ] **TH-015 — Author `turbo-vision.toml`**
  - GREEN: create `internal/tui/themes/turbo-vision.toml` per the spec's Turbo Vision section.
  - Test: `TestEmbeddedTurboVision_Parses` parses it without issues and the parsed theme has `border_style = double`, `window.title.fg = "#ffff55"`, `menubar.shortcut.fg = "#aa0000"`, `menubar.shortcut.underline = false`.

- [ ] **TH-016 — Author `light.toml`**
  - GREEN: create `internal/tui/themes/light.toml` with a light-background palette.
  - Test: `TestEmbeddedLight_Parses` parses without issues; spot-check a few slot values are bright/white-ish.

- [ ] **TH-017 — `//go:embed` the built-in themes**
  - RED: test `TestBuiltinIDs` returns exactly `["default", "light", "turbo-vision"]` (alphabetical).
  - GREEN: in `internal/tui/theme/builtin.go`, embed `themes/*.toml` and expose `BuiltinIDs() []string` and `LoadBuiltin(id string) (*Theme, []Issue, error)`.

## Phase 4: Refactor Inline Color Sites

Promote the ~12 inline `lipgloss.NewStyle().Foreground(Color*)` call sites to fields on `Styles` so live-swap propagates uniformly. This is mechanical but must be done before Phase 5 to avoid stale colors leaking into renders after a theme swap.

- [ ] **TH-018 — Inventory and add `Styles` fields**
  - RED: not applicable (refactor).
  - GREEN: walk the codebase, identify every inline `lipgloss.NewStyle().Foreground(...)` / `BorderForeground(...)` referencing a package-level `Color*` var. For each, either: (a) replace with an existing field on `Styles` if one fits, or (b) add a new field (e.g., `Styles.OverlayBox`, `Styles.Placeholder`) and use it. Inventory list in `specs/theming.md` is the starting point.
  - Run all existing tests; visual output must be unchanged (we're not changing colors yet, just where they're stored).
  - Confirm: `grep -rn "lipgloss.NewStyle" internal/tui/ | grep -v _test.go` shows no remaining theme-relevant inlines (only theme-agnostic ones like `.Reverse(true)`, `.Padding(...)`).

## Phase 5: Live-swap Mechanism

Wire `applyTheme` and `reloadTheme` so theme changes propagate to a running TUI.

- [ ] **TH-019 — `Styles.applyTheme(theme *Theme)`**
  - RED: test `TestStyles_ApplyTheme` constructs a `Styles`, calls `applyTheme(turboVisionTheme)`, then asserts `Styles.Header.GetForeground()` returns the Turbo Vision header foreground color and `Styles.SelectedRow.GetBackground()` returns the Turbo Vision selected background.
  - GREEN: implement `(*Styles).applyTheme(t *theme.Theme)` which re-runs the equivalent of `initBaseStyles()` but pulling values from the theme. The package-level `Color*` vars stay (used as the in-code reference for the *current* theme so inline call sites that haven't been refactored — there shouldn't be any after Phase 4 — would still work; defensive).

- [ ] **TH-020 — `App.reloadTheme(id string) tea.Cmd`**
  - RED: test in `internal/tui/app_test.go`: construct an `App`, call `reloadTheme("turbo-vision")`, assert (a) `app.styles` reflects Turbo Vision colors, (b) the returned `tea.Cmd` produces a `tea.WindowSizeMsg` matching the current width/height, (c) `app.cfg.Theme == "turbo-vision"` and the cfg was saved.
  - Test failure path: `reloadTheme("nonexistent")` returns a cmd that produces a status-bar toast message (Phase 9 wires the actual toast; for now assert the message kind).
  - GREEN: implement `(*App).reloadTheme(id string) tea.Cmd`. Loads the theme (built-in first; user dir wired in Phase 7), calls `app.styles.applyTheme()`, sets `app.cfg.Theme = id`, calls `app.cfg.Save()` (best-effort), returns a cmd emitting `tea.WindowSizeMsg`.

## Phase 6: View → Theme Menu

First user-facing way to switch themes.

- [ ] **TH-021 — Add View top-level menu**
  - RED: test in `internal/tui/menubar_test.go` asserts `defaultMenus()` returns a menu with label `"View"` and shortcut key `'V'`, positioned between `"Edit"` and `"Accounts"`.
  - GREEN: insert the View menu in `defaultMenus()`. Initially contains a single submenu placeholder labeled `"Theme"` with action `MenuActionThemeSubmenu` (new `MenuAction` constant).

- [ ] **TH-022 — Theme submenu population**
  - RED: test that opening the View → Theme submenu populates items from `BuiltinIDs()` (Phase 3) — for now, just built-ins; user dir wired in Phase 7. The active theme has a `✓` prefix in its label.
  - GREEN: extend `MenuBar` to support dynamic submenu population (current `menu` struct is static). Either: (a) make `items` a func returning items, or (b) re-build menus on each open. Option (b) is simpler and only runs on user input.

- [ ] **TH-023 — Selecting a theme triggers `reloadTheme`**
  - RED: test asserts that selecting `"turbo-vision"` from the submenu emits the `tea.Cmd` produced by `reloadTheme("turbo-vision")`.
  - GREEN: wire menu selection in `app.go`'s `handleMenuAction` (or equivalent) to call `a.reloadTheme(themeID)`.

- [ ] **TH-024 — Visual smoke check**
  - Manual: launch `tmoney`, open View → Theme, select Turbo Vision, confirm colors change immediately. Repeat for `light` and `default`. Document any visual issues.

## Phase 7: User Theme Directory Discovery

- [ ] **TH-025 — Discover user themes**
  - RED: test `TestDiscoverUserThemes` with a fixture directory containing `wal.toml` and `mine.toml` returns IDs `["mine", "wal"]`.
  - GREEN: implement `DiscoverUserThemes() ([]string, error)` that scans `~/.config/tmoney/themes/*.toml` (respecting `XDG_CONFIG_HOME`) and returns sorted filename stems. Missing directory is not an error (returns empty list).

- [ ] **TH-026 — User themes override built-ins by ID**
  - RED: test `TestLoadTheme_UserOverride` — with a user dir containing `default.toml` (with a distinct color), `LoadTheme("default")` returns the user version, not the embedded one.
  - GREEN: implement `LoadTheme(id string) (*Theme, []Issue, error)` that checks the user dir first, falls back to embedded.

- [ ] **TH-027 — Submenu lists user themes alongside built-ins**
  - RED: test that the View → Theme submenu, given a fixture user dir, lists both built-ins and user themes. Active theme marker still works.
  - GREEN: update Phase 6's submenu builder to include user themes.

## Phase 8: Persistence

- [ ] **TH-028 — Add `Theme` field to `config.Config`**
  - RED: test that an existing `config.json` without a `theme` key loads with `cfg.Theme == ""`. Test that saving a config with `Theme = "turbo-vision"` and reloading produces the same value.
  - GREEN: add `Theme string \`json:"theme,omitempty"\`` to `Config`.

- [ ] **TH-029 — Apply persisted theme on startup**
  - RED: test that `App` constructed with `cfg.Theme = "turbo-vision"` has Turbo Vision colors applied to `app.styles`.
  - GREEN: in `NewApp`, after `NewStyles()`, if `cfg.Theme != ""`, call `app.styles.applyTheme(loadedTheme)`. If the configured theme fails to load, fall back to default (issue surfaced by Phase 9).

## Phase 9: Error Surfacing

- [ ] **TH-030 — Log file infrastructure**
  - RED: test `TestLogTheme_AppendsEntries` writes two issues, reads the file back, asserts both timestamped lines are present.
  - GREEN: introduce `internal/log/` package with `LogPath()` returning `~/.config/tmoney/log.txt` (XDG-aware) and `Append(category, message string) error`. Append-only, plain text, RFC3339 timestamps.

- [ ] **TH-031 — Status-bar toast component**
  - RED: test that adding a toast to the status bar renders in the status bar area for the configured duration, then disappears on the next render after the timeout.
  - GREEN: extend `StatusBar` with an optional transient message field plus a tea.Cmd that clears it after ~5s. Toast persists across re-renders within its window.

- [ ] **TH-032 — Wire theme-load issues to log + toast**
  - RED: test that `reloadTheme("brokenfile")` (where the user-dir theme has a malformed value) appends an entry to the log file and emits a toast message describing the issue count.
  - GREEN: connect Phase 2's `Issue` list output to the log writer; when issues exist, emit a toast `Theme '<id>': <N> issues, see <log path>`. Successful loads with zero issues do not toast.

## Phase 10: Pywal Helper

- [ ] **TH-033 — `colors.json` reader**
  - RED: test `TestReadWalColors_Sample` reads a fixture `testdata/wal-sample-colors.json` and returns a struct with `Special.Background == "#1d1f21"` and `Colors.Color3 == "#fabd2f"` (or whatever the sample contains).
  - Test missing file returns the documented error message.
  - GREEN: implement `internal/cli/wal.go` with a struct and JSON unmarshal mirroring pywal's `colors.json` shape.

- [ ] **TH-034 — Pywal → theme TOML converter**
  - RED: test `TestWalToTheme_GeneratesExpectedTOML` runs the converter on the fixture and asserts the produced TOML contains the expected slot mappings per the table in `specs/theming.md`. Includes the comment header.
  - GREEN: implement `walToThemeTOML(walColors *WalColors, sourcePath string, ts time.Time) string`. Plain string concatenation is fine for output; we control the format.

- [ ] **TH-035 — `tmoney theme` parent command**
  - RED: test `tmoney theme --help` lists `list` and `generate-from-wal` as subcommands.
  - GREEN: implement `internal/cli/theme.go` registering the `theme` parent subcommand on the root.

- [ ] **TH-036 — `tmoney theme generate-from-wal` subcommand**
  - RED: test invocations:
    - default writes to `~/.config/tmoney/themes/wal.toml` (use a temp HOME for the test).
    - `--output -` writes to stdout.
    - `--output /tmp/foo.toml` writes to that path.
    - missing pywal cache produces the error message and exit code 1.
  - GREEN: implement `internal/cli/theme_wal.go`. Uses `filepath.Join(os.UserHomeDir(), ".cache/wal/colors.json")` (XDG_CACHE_HOME aware).

## Phase 11: `tmoney theme list`

- [ ] **TH-037 — `tmoney theme list` subcommand**
  - RED: test that `tmoney theme list` with a fixture user dir prints a table with columns `ID`, `SOURCE`, `NAME`, `ACTIVE`. Built-ins appear with source `built-in`; user themes with source `user`. The current theme (read from config) has `*` in the ACTIVE column.
  - GREEN: implement `internal/cli/theme_list.go`. Uses Go's `text/tabwriter` for column alignment.

## Phase 12: Documentation

- [ ] **TH-038 — README updates**
  - GREEN: add a `## Themes` section to `README.md` documenting:
    - The three built-in themes with one-sentence descriptions.
    - How to switch via View → Theme.
    - How to author a custom theme (`~/.config/tmoney/themes/`, link to `specs/theming.md` for the slot list).
    - The pywal helper invocation and the suggested `~/.config/wal/postrun.sh` hook.
    - The behavior on a misconfigured theme (toast + log file).
  - Add a `## CLI` section update mentioning the noun-verb subcommand structure is in progress; for now, `tmoney version`, `tmoney theme list`, `tmoney theme generate-from-wal` are the only Cobra-native commands; legacy `--flag` style still works for everything else.

- [ ] **TH-039 — Mark spec files as implemented**
  - GREEN: add a status note at the top of `specs/theming.md` and `specs/cli-router.md` indicating the v1 / scaffold portions are implemented and pointing to the next migration batches.

---

## Out of Scope

The following are explicitly deferred — not in this implementation plan:

- Migration of the remaining ~50 legacy flags to Cobra subcommands (see `specs/cli-router.md` Migration Strategy section for batch sequencing).
- Solid-background fill (`desktop.bg` painting).
- OSC color queries / runtime auto-detect.
- Per-region border styles.
- File-watch auto-reload.
- Adaptive light/dark themes.
- `tmoney theme show <id>`, `tmoney theme validate <path>`.
