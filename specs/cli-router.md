# CLI Router Specification

> **Status (2026-05-09):** migration complete. Every legacy `--flag` verb has been ported to a Cobra noun-verb subcommand; `parseArgs`, `RunLegacy`, and `cliOptions` have all been deleted. The Cobra root in `internal/cli/root.go` is the only entry point. The post-migration CLI reference lives in [`specs/cli.md`](cli.md); this document is preserved for design context. The original phased plan is retained at the bottom under [History](#history).

## Overview

TMoney's current CLI exposes ~50 verbs as flat top-level flags (`--add-account`, `--backup`, `--buy`, `--import`, etc.) parsed in `cmd/tmoney/main.go`'s `run()` function. This specification replaces that flag-based dispatch with a Cobra-based subcommand router using a noun-verb taxonomy (`tmoney account add`, `tmoney db backup`, `tmoney investment buy`).

This spec is a prerequisite for the theming feature ([`specs/theming.md`](theming.md)), which introduces the first Cobra subcommands (`tmoney theme list`, `tmoney theme generate-from-wal`). The full migration of all existing verbs is in scope but explicitly opportunistic — verbs migrate in batches as part of separate work, not all-at-once with theming.

## Goals

- Move from flat-flag dispatch to Cobra subcommands.
- Adopt a noun-verb taxonomy aligned with the existing `internal/<domain>/` package structure (`account`, `transaction`, `security`, etc.).
- Preserve `tmoney` (no args) → TUI launch; preserve `tmoney <file.tdb>` → TUI with that file.
- Provide auto-generated nested help (`tmoney --help`, `tmoney account --help`, `tmoney account add --help`).
- Move `main.go` to the module root; place command implementations in `internal/cli/`.

## Non-goals

- **Backward compatibility for legacy flag forms.** Once a verb is migrated to Cobra, the old `--flag` form is removed in the same change. No deprecation period. (User explicitly opted into this; one CLI style is simpler than two.)
- **Aliases for common verbs.** No `tmoney portfolio` shortcut for `tmoney investment portfolio` in v1. Add only if users ask.
- **Shell completion installation UX.** Cobra generates completion scripts (`tmoney completion bash|zsh|fish`); we expose the subcommand but don't ship installers.
- **Migrating all 50 verbs in one PR.** Theming requires only the CLI scaffold + `theme` and `version` verbs. The remaining verbs migrate in subsequent batches.

## Layout

Single-binary repo convention: `main.go` at the module root, command implementations in `internal/cli/`.

```
/tmoney/
├── main.go                     # 5-10 line entry: calls cli.Execute()
├── internal/
│   ├── cli/                    # all command code
│   │   ├── root.go             # cobra root command + global flags
│   │   ├── tui.go              # default no-args path: launch TUI
│   │   ├── version.go
│   │   ├── theme.go            # `tmoney theme` parent command
│   │   ├── theme_list.go
│   │   ├── theme_wal.go
│   │   ├── account.go          # noun parents
│   │   ├── account_add.go      # verb files (one per leaf command)
│   │   ├── account_list.go
│   │   ├── ...
│   │   └── *_test.go
│   ├── tui/
│   ├── config/
│   └── ... (existing internal packages)
├── go.mod
└── Makefile
```

**Why `main.go` at root + `internal/cli/`** (not `cmd/tmoney/cmd/` and not `cmd/` at the top level):

- Single-binary projects place `main.go` at the root by convention (Terraform, Hugo, fzf, gopls).
- `internal/cli/` correctly signals "private to this module" and names the role precisely. Common alternatives: `internal/cmd/`, `internal/command/`. `cli` is the most common in newer Go projects.
- `cmd/<binary>/cmd/` is Cobra's historical scaffold default but reads as "double counting" in repos that already use `cmd/<binary>/`.
- A top-level `cmd/` repurposed as a non-main Go package violates the existing convention (`cmd/` traditionally contains main packages for binaries).

**File migration:**

- `cmd/tmoney/main.go` → split: 5-line entry at `/main.go`, rest into `internal/cli/root.go` + `internal/cli/tui.go`.
- `cmd/tmoney/args.go` (29K), `cmd/tmoney/commands.go` (93K), `cmd/tmoney/format.go` (28K), `cmd/tmoney/update_prices.go` (3K), `cmd/tmoney/*_test.go` → all moved to `internal/cli/`.
- `cmd/tmoney/some-file.tdb` (test artifact in git status) — review and either gitignore or move into a `testdata/` directory.

**Build line:** `go build -o tmoney .` (root package). Update Makefile accordingly.

## Library

**Cobra** (`github.com/spf13/cobra`).

Reasons:
- De facto standard for Go subcommand CLIs (`gh`, `kubectl`, `hugo`, `helm`).
- Auto-generates nested `--help` for every command.
- Persistent flags, positional args, and shell completion built-in.
- Strong community; well-trodden patterns.
- ~300KB binary growth is rounding error on a 71MB binary.

Alternatives considered and rejected: `urfave/cli` (smaller community, fewer features), stdlib `flag` (would re-invent help/completion), `kong` (niche).

## Default Behavior

- `tmoney` → launches TUI (existing behavior preserved).
- `tmoney <file.tdb>` → launches TUI with that file.
- `tmoney --help` → prints top-level help with the subcommand list.
- `tmoney --version` → prints version. (Equivalent to `tmoney version`.)
- Unknown subcommand → Cobra default error + suggestions.

The root Cobra command's `RunE` handles the "no subcommand, possibly a positional file path" case by delegating to the existing TUI launch logic.

## Global Flags

| Flag | Short | Scope | Description |
|---|---|---|---|
| `--file <path>` | `-f` | persistent (all commands) | Database file. |
| `--help` | `-h` | persistent | Show help (auto-provided by Cobra). |
| `--version` | `-v` | root only | Print version (auto-provided by Cobra). |

The `--file`/`-f` flag is persistent — every subcommand that operates on a database respects it.

Per-command flags are documented under each command in **Command Tree**.

## Command Tree

Final taxonomy (target end state of full migration). Verbs marked `[v1]` are required for the theming feature; the rest migrate in subsequent batches.

```
tmoney                                      # launch TUI
tmoney version                              [v1]
tmoney completion bash|zsh|fish             # auto-provided by Cobra

tmoney theme list                           [v1]
tmoney theme generate-from-wal              [v1]

tmoney db create <path>
tmoney db backup
tmoney db restore <backup-path>
tmoney db list-backups

tmoney account add ...
tmoney account list
tmoney account show <name>
tmoney account balance

tmoney transaction add ...
tmoney transaction list
tmoney transaction void <id>
tmoney transaction search <term>

tmoney transfer add ...
tmoney transfer link

tmoney scheduled add ...
tmoney scheduled post <id>
tmoney scheduled skip <id>
tmoney scheduled list

tmoney reconcile start
tmoney reconcile mark <id>...
tmoney reconcile finish
tmoney reconcile status

tmoney security add ...
tmoney security list
tmoney security show <ticker>
tmoney security edit <ticker>
tmoney security hide <ticker>
tmoney security unhide <ticker>
tmoney security delete <ticker>

tmoney price add ...
tmoney price list <ticker>
tmoney price current <ticker>
tmoney price import <file>
tmoney price update

tmoney investment buy ...
tmoney investment sell ...
tmoney investment dividend ...
tmoney investment reinvest ...
tmoney investment fee ...
tmoney investment deposit ...
tmoney investment withdraw ...
tmoney investment transfer ...
tmoney investment split ...
tmoney investment merge ...
tmoney investment spin-off ...
tmoney investment portfolio

tmoney import <file>            # transactions (top-level since there's only one
                                # general "import")
tmoney export <file>

tmoney report net-worth
tmoney report spending
```

### Mapping table: legacy flag → new subcommand

This table is the migration reference. Each row is a single PR's worth of work (or batched with siblings under the same noun group).

| Legacy flag | New subcommand |
|---|---|
| `--create <path>` | `tmoney db create <path>` |
| `--backup` | `tmoney db backup` |
| `--restore <path>` | `tmoney db restore <path>` |
| `--list-backups` | `tmoney db list-backups` |
| `--add-account` (with sub-flags) | `tmoney account add ...` |
| `--list-accounts` | `tmoney account list` |
| `--account <name>` | `tmoney account show <name>` |
| `--balance` | `tmoney account balance` |
| `--add-transaction` | `tmoney transaction add` |
| `--transactions` | `tmoney transaction list` |
| `--void <id>` | `tmoney transaction void <id>` |
| `--search <term>` | `tmoney transaction search <term>` |
| `--transfer` | `tmoney transfer add` |
| `--link-transfers` | `tmoney transfer link` |
| `--add-scheduled` | `tmoney scheduled add` |
| `--post-scheduled <id>` | `tmoney scheduled post <id>` |
| `--skip-scheduled <id>` | `tmoney scheduled skip <id>` |
| `--scheduled` | `tmoney scheduled list` |
| `--start-reconcile` | `tmoney reconcile start` |
| `--mark-reconciled <id>...` | `tmoney reconcile mark <id>...` |
| `--finish-reconcile` | `tmoney reconcile finish` |
| `--reconcile-status` | `tmoney reconcile status` |
| `--list-securities` | `tmoney security list` |
| `--add-security` | `tmoney security add` |
| `--security <ticker>` | `tmoney security show <ticker>` |
| `--edit-security <ticker>` | `tmoney security edit <ticker>` |
| `--hide-security <ticker>` | `tmoney security hide <ticker>` |
| `--unhide-security <ticker>` | `tmoney security unhide <ticker>` |
| `--delete-security <ticker>` | `tmoney security delete <ticker>` |
| `--prices <ticker>` | `tmoney price list <ticker>` |
| `--add-price` | `tmoney price add` |
| `--current-price <ticker>` | `tmoney price current <ticker>` |
| `--import-prices <file>` | `tmoney price import <file>` |
| `--update-prices` | `tmoney price update` |
| `--buy` | `tmoney investment buy` |
| `--sell` | `tmoney investment sell` |
| `--dividend` | `tmoney investment dividend` |
| `--reinvest` | `tmoney investment reinvest` |
| `--investment-fee` | `tmoney investment fee` |
| `--invest-deposit` | `tmoney investment deposit` |
| `--invest-withdraw` | `tmoney investment withdraw` |
| `--transfer-shares` | `tmoney investment transfer` |
| `--split` | `tmoney investment split` |
| `--merge-security` | `tmoney investment merge` |
| `--spin-off` | `tmoney investment spin-off` |
| `--portfolio` | `tmoney investment portfolio` |
| `--import <file>` | `tmoney import <file>` |
| `--export <file>` | `tmoney export <file>` |
| `--report` (with `--report-type`) | `tmoney report net-worth` / `tmoney report spending` |

## Help Behavior

Cobra auto-generates help from each command's `Use`, `Short`, `Long`, `Example`, and flag definitions. Required conventions for every command:

- **`Use`**: the command pattern, e.g., `add <name>`.
- **`Short`**: one-line description (≤ 60 chars). Shows in parent's command list.
- **`Long`**: optional, multi-paragraph. Shows on `<cmd> --help`.
- **`Example`**: at least one realistic invocation example.

Top-level help lists noun groups grouped logically. Sub-noun help (`tmoney account --help`) lists verbs for that noun.

## Test Strategy

### Unit tests

- **Per-command argument parsing**: each leaf command has a test verifying that valid args produce the expected handler invocation and that invalid args produce the documented error.
- **Root command no-args path**: `tmoney` (no args) routes to TUI launcher; `tmoney <file.tdb>` routes to TUI launcher with file set; `tmoney --file foo.tdb` likewise.
- **Help text**: a smoke test asserts each command has non-empty `Short` and at least one `Example`.

### Migration regression tests

- For each batch, the pre-existing `commands_test.go` tests for that group are migrated 1:1: same scenario, same expected output, just constructed with the new Cobra commands instead of `parseArgs`. No coverage drop.

### Integration tests

- `tmoney --help` exits 0 and includes every top-level noun group.
- `tmoney <noun> --help` exits 0 and includes every verb in that group.
- `tmoney completion bash` produces non-empty output (smoke test for Cobra wiring).

## Risks & Open Questions

- **Cobra option style for complex `add` commands**: `tmoney transaction add` has many fields. Cobra supports both flag-based (`--account`, `--amount`, `--payee`) and positional argument forms. Recommendation: flag-based, mirroring the legacy `--add-transaction` flag style. Decision deferred to per-batch implementation.
- **Persistent flag inheritance**: `--file` is persistent at root, but some commands genuinely don't need a database (`version`, `theme list`, `completion`). Cobra allows per-command flag exclusion; verify no DB connection is opened unnecessarily.
- **Test artifact `cmd/tmoney/some-file.tdb`**: visible in git status; needs disposition (gitignore vs delete vs move to `testdata/`). Address in batch 1.

## History

The migration is complete. The following section is preserved for posterity — it describes the phased plan that was executed, not the current state.

### Migration Strategy

The full port was split into **batches**, each of which was its own implementation plan and PR set:

1. **Scaffold + theme + version** (covered by this spec and the theming implementation plan): introduce Cobra root, move `main.go`, set up `internal/cli/`, add `theme list`, `theme generate-from-wal`, `version`. No legacy flag was removed in this batch — the existing `parseArgs` path remained for unmigrated verbs (the only point where two CLI styles temporarily coexisted).
2. **`db` group**: `db create`, `db backup`, `db restore`, `db list-backups`. Removed corresponding legacy flags.
3. **`account` group**.
4. **`transaction` group**.
5. **`scheduled` group**.
6. **`reconcile` group**.
7. **`security` group**.
8. **`price` group**.
9. **`investment` group** (largest single batch — 12 verbs).
10. **`import`, `export`, `report`** (residual top-level verbs).

Each batch:
- Added the new subcommand(s).
- Removed the legacy `--flag` and its handler.
- Updated README examples for the migrated verbs.

After batch 10, `parseArgs` and the flag-flat dispatch in `run()` were deleted. The detailed per-verb plan and ticket numbers (CM-001 through CM-068) are recorded in [`specs/implementation-plan-cli-cobra-migration.md`](implementation-plan-cli-cobra-migration.md).

### Disposition of `specs/cli.md`

`specs/cli.md` was rewritten once at the end of the migration (CM-067) to be the canonical, post-migration reference; this design spec is retained for context only.

## References

- Cobra: https://github.com/spf13/cobra
- Go project layout discussion: https://github.com/golang-standards/project-layout (note: the `cmd/` convention there is for *multi*-binary repos; single-binary repos use `main.go` at root)
