# Theme System Specification

## Overview

TMoney's TUI currently uses a single hard-coded color palette in `internal/tui/styles.go`. This spec defines a skinnable theme system that lets users choose between built-in themes (including a faithful Turbo Vision look) and define their own theme files. A pywal helper subcommand generates a theme from the user's system color scheme for Omarchy/pywal users.

## Goals

- Ship three built-in themes: `default` (current look), `turbo-vision` (Borland blue + double borders + yellow titles + red shortcut keys), and `light` (readable on light terminals).
- Support user-authored themes in `~/.config/tmoney/themes/*.toml`. User themes with the same ID as a built-in override the built-in.
- Live theme switching from a new **View → Theme** menu, with selection persisted in `~/.config/tmoney/config.json`.
- Provide `tmoney theme generate-from-wal` to produce a theme from `~/.cache/wal/colors.json`.
- Hand-editable theme files: TOML, optional slots, fall back to built-in `default` for anything missing.

## Non-goals

- Runtime auto-detection of terminal background or system colors (OSC color queries, `$COLORFGBG` parsing). Out of scope; pywal helper is the documented path for system-derived themes.
- Per-region border style overrides. One global `border_style` per theme; per-region can be added later if needed.
- Layout knobs (padding, drop-shadows, full-screen background fill). The Turbo Vision theme will look "Turbo-Vision-ish" via colors and double borders but will not paint a solid blue desktop fill in v1. Promotion to layout-level skinning is explicitly deferred (scope expansion path noted in **Future Work**).
- Adaptive themes (auto-switch on light/dark terminal). Themes are explicit choices.

## Scope

A theme defines:

- ~27 named **colors**, addressed via flat dotted-key slots (e.g., `window.border.fg`).
- One **border style** for the whole theme (`single`, `double`, `rounded`, `thick`).
- Three minor **symbols** (menu separator, focus indicator, checkmark).
- Two boolean **shortcut display options** (`menubar.shortcut.underline`, `statusbar.shortcut.underline`) so themes can choose underline-only, color-only, both, or neither for keyboard shortcut letters.
- **Metadata**: `name`, `description`.

A theme does **not** define padding, dialog geometry, table column widths, layout breakpoints, or any non-color/non-border styling.

## Theme File Format

TOML. Hand-editable; comments allowed; flat dotted keys.

### Schema

```toml
# Metadata
name = "Turbo Vision"
description = "Classic Borland Turbo Vision blue + double borders"

# Reserved for future schema migrations. Parser ignores it in v1.
# version = 1

# Global border style applied to all bordered elements (windows, dialogs,
# tables, sidebar). One of: "single", "double", "rounded", "thick".
border_style = "double"

# ---- Desktop / app chrome ----
# Empty string on a *.bg slot means "transparent" — terminal background shows
# through. Any non-empty value paints that background explicitly.
desktop.bg               = ""           # not painted in v1; reserved for future
                                        # full-screen-fill support.
menubar.fg               = "#000000"
menubar.bg               = "#aaaaaa"
menubar.active.fg        = "#ffffff"    # highlighted top-level menu item
menubar.active.bg        = "#000000"
menubar.shortcut.fg      = "#aa0000"    # red letter ("F" in "File"); optional —
                                        # if unset, inherits menubar.fg.
menubar.shortcut.underline = true       # underline shortcut letter (default true)
statusbar.fg             = "#000000"
statusbar.bg             = "#aaaaaa"
statusbar.shortcut.fg    = "#aa0000"    # optional; inherits statusbar.fg if unset
statusbar.shortcut.underline = true

# ---- Windows / panels (main content area) ----
window.bg                = ""           # transparent in default; "#0000aa" in TV
window.fg                = "#aaaaaa"
window.border.fg         = "#ffffff"
window.title.fg          = "#ffff55"

# ---- Dialogs ----
dialog.bg                = "#aaaaaa"
dialog.fg                = "#000000"
dialog.border.fg         = "#000000"
dialog.title.fg          = "#000000"

# ---- Tables ----
table.header.fg          = "#ffff55"
table.row.fg             = "#aaaaaa"
table.selected.fg        = "#000000"
table.selected.bg        = "#00aaaa"

# ---- Semantic text ----
text.positive            = "#55ff55"    # gains, income
text.negative            = "#ff5555"    # losses, expenses
text.alert               = "#ffff55"    # due/upcoming items
text.muted               = "#5555ff"    # dim secondary text
text.title               = "#ffffff"    # section titles
text.error               = "#ff5555"    # error messages

# ---- Symbols ----
symbols.menu_separator   = " │ "
symbols.focus_indicator  = "▶ "
symbols.checkmark        = "✓"
```

### Color value formats

Each color slot accepts either:

- **Hex RGB**: `"#rrggbb"` (case-insensitive, 6 hex digits, leading `#` required).
- **ANSI 256 number as string**: `"0"` through `"255"`.

Anything else is a parse error for that slot.

Empty string `""` on `*.bg` and `*.fg` slots means "do not set this property" — the terminal default (transparent) is used. This is how the `default` theme retains today's transparent-background look.

### Required vs optional slots

**All slots are optional.** A user theme may specify only the slots it wants to override; everything else falls back to the built-in `default` theme's value. Minimal valid user theme:

```toml
name = "Just Red Negatives"
text.negative = "#ff0000"
```

The two metadata fields (`name`, `description`) are also optional. If `name` is missing, the filename stem is used as the display name.

### Schema versioning

No `version` field is required in v1. Adding new optional slots is naturally backward-compatible (missing keys fall back to default). The key `version` is reserved — the parser will recognize it and ignore it in v1, keeping the door open for explicit version negotiation in v2+.

## Theme File Locations

Theme files are loaded from two sources, in this order:

1. **Embedded built-ins** (compiled into the binary via `//go:embed`):
   - `internal/tui/themes/default.toml`
   - `internal/tui/themes/turbo-vision.toml`
   - `internal/tui/themes/light.toml`

2. **User directory**: `~/.config/tmoney/themes/*.toml` (or `$XDG_CONFIG_HOME/tmoney/themes/*.toml` if set).

The **theme ID** is the filename stem (`turbo-vision.toml` → `turbo-vision`). If a user theme has the same ID as a built-in, the user theme wins. This lets users fork built-in themes by copying them into the user dir and editing.

## Built-in Themes

### `default`

Reproduces the existing color palette in `internal/tui/styles.go`. Background slots empty (transparent passthrough). Border style `single`. Shortcut underline on, no shortcut color.

This theme is the fallback for missing slots in any user theme. It must remain stable; any change to its colors is a user-visible change to the app's default look.

### `turbo-vision`

Faithful to the Borland Turbo Vision aesthetic:

- Border style: `double`
- Menu bar: black on light gray, with red shortcut letters (`menubar.shortcut.fg = "#aa0000"`, underline off)
- Window: yellow titles on blue (`window.bg = "#0000aa"`, `window.title.fg = "#ffff55"`, `window.border.fg = "#ffffff"`)
- Dialogs: black on light gray, dark borders
- Selected row: black on cyan (`table.selected.bg = "#00aaaa"`)
- Semantic text: bright variants — `#55ff55` positive, `#ff5555` negative, `#ffff55` alert, `#5555ff` muted

Limitation acknowledged in v1: Turbo Vision's solid-blue desktop fill is *not* implemented (would require `desktop.bg` painting, which is a layout-level change deferred to a future scope expansion). Window content areas will show terminal-default behind any unbordered region.

### `light`

A readable theme for light-background terminals. Dark text on near-white panels, single borders, muted gray for de-emphasized text. Designed to fix the gray-on-gray status bar contrast issue that the current `default` palette has on light terminals.

## Theme Switching UX

### Menu

A new top-level **View** menu is added to the menu bar (between **Edit** and **Accounts**, shortcut letter `V`). It contains:

- **Theme** → submenu listing all available themes (built-ins + user dir, alphabetical, with a checkmark `✓` on the active theme). Selecting a theme applies and persists it immediately.

The View menu is reserved for future visual/display preferences (column visibility, density, etc.); only **Theme** lives there in v1.

### Live-swap

Selecting a theme rebuilds `app.styles` and forces a re-layout via a synthetic `tea.WindowSizeMsg` so all width-dependent styles regenerate. No restart required.

### Persistence

A new `Theme string` field is added to `internal/config/Config`. On startup, the configured theme ID is loaded; if missing or fails validation, the app falls back to `default` and surfaces the error via the status-bar toast (see **Validation & Error Handling**). An empty string means "use default."

### Hotkey

No keyboard shortcut for theme switching in v1. Users access themes via the View menu only.

## Live-swap Implementation

Audit findings on the existing codebase:

- All component `Render(styles Styles, ...)` methods take the current `Styles` snapshot as a parameter at render time, reading fresh from `app.styles`. No struct outside `Styles` itself caches `lipgloss.Style` fields.
- ~12 sites use inline `lipgloss.NewStyle().Foreground(ColorMuted)` / `.BorderForeground(ColorBorder)` referencing package-level `Color*` vars (in `app.go`, `dialog.go`, `split_dialog.go`, `corporate_action_history.go`, `corporate_action_merger_confirm.go`). These must be promoted into proper fields on `Styles` (e.g., `Styles.OverlayBox`, `Styles.PlaceholderText`) and call sites updated.
- Theme-agnostic inlines like `lipgloss.NewStyle().Reverse(true)` need no change.

The reload mechanism in `App`:

```go
func (a *App) reloadTheme(id string) tea.Cmd {
    theme, err := loadTheme(id)
    if err != nil {
        return showThemeErrorToast(err)
    }
    a.styles.applyTheme(theme)
    a.styles.Resize(a.width, a.height)
    a.cfg.Theme = id
    _ = a.cfg.Save() // best-effort
    return func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
}
```

`Styles.applyTheme(theme)` re-runs `initBaseStyles()` using the theme's slot values. The synthetic `WindowSizeMsg` triggers every component's normal resize path, which reads from `app.styles` and produces the new layout on the next render.

When activating a theme by ID, the loader always re-reads from disk (does not cache theme TOML in memory beyond the current render). This means re-running the pywal helper while the TUI is open and re-selecting `wal` from the menu picks up the new colors.

## Validation & Error Handling

**Strategy:** hybrid — strict on values, lenient on keys.

- **Malformed value** for a known slot (bad hex, non-numeric ANSI 256, out-of-range number, unrecognized `border_style` value): that slot falls back to the `default` theme's value; an error is logged with the specific key and offending value.
- **Unknown key**: logged as a warning; ignored. (Catches typos like `windows.bg` instead of `window.bg`.)
- **Missing required structural element** (theme file is unparseable TOML): the entire theme is rejected; load falls back to `default`; an error toast surfaces the failure.

### Error surfacing

- A **status-bar toast** appears for ~5 seconds when a theme load has issues:
  - `Theme 'wal': 2 issues, see ~/.config/tmoney/log.txt`
- **Log file**: a new `~/.config/tmoney/log.txt` (or `$XDG_CONFIG_HOME/tmoney/log.txt`) is created on first error. Append-only. Each entry is timestamped and includes the theme ID, slot key, and reason. Rotated only on user request (no automatic rotation in v1).

The log file is also used for any other non-fatal error in the future. For v1 it's theme-error-only, but the location is general-purpose.

## Pywal Helper

### Subcommand

```
tmoney theme generate-from-wal [--output PATH]
```

- Reads `~/.cache/wal/colors.json` (the standard pywal output path).
- If absent, exits with: `pywal cache not found at ~/.cache/wal/colors.json — is pywal installed and has it run?` (exit code 1).
- Default output: `~/.config/tmoney/themes/wal.toml` (creates the directory if needed).
- `--output -` writes to stdout.
- `--output PATH` writes to a custom path.

### Generated theme

Includes a comment header:

```toml
# Generated from ~/.cache/wal/colors.json on 2026-05-03T14:22:01Z.
# Re-run `tmoney theme generate-from-wal` to regenerate after pywal updates.
# Live-swap is not automatic — re-select 'wal' in View → Theme to apply.
name = "wal"
description = "Generated from pywal palette"
border_style = "single"
...
```

### Slot mapping

| TMoney slot | pywal source |
|---|---|
| `desktop.bg` | `special.background` (commented out — not painted in v1) |
| `window.bg` | `special.background` |
| `window.fg` | `special.foreground` |
| `window.border.fg` | `colors.color7` |
| `window.title.fg` | `colors.color3` |
| `menubar.bg` | `colors.color0` |
| `menubar.fg` | `colors.color7` |
| `menubar.active.bg` | `colors.color4` |
| `menubar.active.fg` | `special.background` |
| `menubar.shortcut.fg` | `colors.color1` |
| `statusbar.bg` | `colors.color0` |
| `statusbar.fg` | `colors.color7` |
| `dialog.bg` | `colors.color8` |
| `dialog.fg` | `special.foreground` |
| `dialog.border.fg` | `colors.color7` |
| `dialog.title.fg` | `colors.color3` |
| `table.header.fg` | `colors.color3` |
| `table.row.fg` | `special.foreground` |
| `table.selected.bg` | `colors.color4` |
| `table.selected.fg` | `special.background` |
| `text.positive` | `colors.color2` |
| `text.negative` | `colors.color1` |
| `text.alert` | `colors.color3` |
| `text.muted` | `colors.color8` |
| `text.title` | `special.foreground` |
| `text.error` | `colors.color1` |
| `border_style` | `single` (constant; pywal has no opinion) |

Symbol slots and shortcut display options are not derived from pywal; the helper omits them so they fall back to default.

### Pywal post-hook (documented, not automated)

The README will document a one-line pywal post-hook for users who want auto-regen on wallpaper change:

```bash
# Add to ~/.config/wal/postrun.sh (or equivalent), make executable
tmoney theme generate-from-wal
```

The TUI does not auto-pick-up the regenerated file; users must re-select `wal` from View → Theme. (File-watch auto-reload is a possible v2 enhancement.)

## CLI Surface

The theming feature requires a Cobra-based CLI router; that's specified separately in [`specs/cli-router.md`](cli-router.md). The theme-related subcommands are:

```
tmoney theme list                   # list available themes (built-ins + user dir)
tmoney theme generate-from-wal      # see above
```

`tmoney theme list` output:

```
ID             SOURCE     NAME                  ACTIVE
default        built-in   Default
turbo-vision   built-in   Turbo Vision
light          built-in   Light
wal            user       wal                   *
```

## Test Strategy

### Unit tests

- **Theme parser** (`internal/tui/theme/theme_test.go`):
  - Valid theme parses correctly; all slots populated.
  - Malformed hex value → slot falls back to default; warning logged.
  - Out-of-range ANSI 256 → fallback + warning.
  - Unknown key → warning logged; theme still loads.
  - Empty `name` → falls back to filename stem.
  - Unparseable TOML → load returns error.
  - Empty string on `*.bg` slots is preserved as "transparent."
- **Pywal mapping** (`internal/cli/theme_wal_test.go`):
  - Sample `colors.json` produces the expected TOML output.
  - Missing `colors.json` returns the documented error.
  - `--output -` writes to stdout, not the file.

### Golden tests

- For each built-in theme (`default`, `turbo-vision`, `light`), render a representative full screen (dashboard view + open dialog) and snapshot the ANSI output. Snapshot files in `internal/tui/testdata/themes/<theme>.golden`. Regression test catches unintended visual drift.

### Integration tests

- Selecting a theme from the View → Theme submenu rebuilds `app.styles` and the next render reflects the new colors. Verify by selecting `turbo-vision`, then asserting that `app.styles.Header.GetForeground()` returns the Turbo Vision header color.
- Theme persistence: after selecting a theme, `config.Config.Theme` is set, `Save()` is called, and a fresh `App` constructed from the saved config loads the same theme.
- Bad theme load surfaces a status-bar toast and falls back to `default`.

## Refactor Prerequisites

Before live-swap can work cleanly, the inline `lipgloss.NewStyle().Foreground(Color*)` call sites must be promoted to `Styles` fields. Inventory:

| File | Purpose | New `Styles` field |
|---|---|---|
| `app.go:3461` | Loading-dashboard placeholder | `Styles.Placeholder` |
| `app.go:3503` | Sidebar separator | (covered by existing `Sidebar`) |
| `app.go:3864` | Reports placeholder | `Styles.Placeholder` |
| `app.go:3917` | Loading message | `Styles.Placeholder` |
| `app.go:3956`, `3976`, `4068`, `4079`, `4088`, `4127`, `4135` | Various inline styles | enumerate during refactor |
| `corporate_action_history.go:183` | Overlay box border | `Styles.OverlayBox` |
| `corporate_action_merger_confirm.go:268` | Overlay box border | `Styles.OverlayBox` (shared) |
| `dialog.go:771` | Placeholder text in field | `Styles.Placeholder` |
| `split_dialog.go:520` | Placeholder text in field | `Styles.Placeholder` |

The exact list will be finalized during implementation; the implementation plan tracks this as Phase 4.

## Out of Scope / Future Work

- **Solid-background fill** (`desktop.bg` actually painted across the viewport). Required for full Turbo Vision fidelity. Defer until v1 user feedback indicates it's needed.
- **OSC color queries** for runtime auto-detection of terminal palette. The pywal helper covers the loud case; OSC is the right next step if more users want auto-detect on non-pywal systems.
- **Per-region border styles** (e.g., dialogs use `rounded` while windows use `double`). Add a slot when there's a concrete user request.
- **File-watch auto-reload**: detect changes to the active theme file and live-swap automatically. Currently users must re-select via the View menu.
- **Adaptive themes**: light/dark variant pairing. Would need an `appearance` field and a way to consult terminal background.
- **`tmoney theme show <id>`**, **`tmoney theme validate <path>`**: useful but not load-bearing for the v1 feature.
