package transfer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This file is the guard that keeps the door shut.
//
// specs/design-unified-transfer.md replaced four transfer create paths, two edit
// paths and two delete paths with one owner. Nothing in the compiler prevents
// someone from adding a fifth — a helper on transaction.Service that writes two
// rows, or a new investment.Service method that mints a counterpart. These tests
// fail if that happens.

// repoRoot walks up from this package to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate the module root (no go.mod found walking up)")
	return ""
}

// walkProductionGo visits every non-test .go file under internal/ and cmd/.
func walkProductionGo(t *testing.T, fn func(relPath string, body string)) {
	t.Helper()
	root := repoRoot(t)
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(root, sub)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			rel, _ := filepath.Rel(root, path)
			fn(rel, string(b))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", sub, err)
		}
	}
}

// TestArch_NoResurrectedTransferMethods fails if any of the methods phase 5
// deleted comes back.
//
// The list starts with NO allowlist, which is only possible because every call
// site was migrated first: 22 non-test sites existed before phase 3 (cli/transfer
// 7, undo 9, scheduled 2, tui 1, plus 3 internal UpdateTransferCash self-calls),
// and all of them are now either the transfer owner or deleted.
func TestArch_NoResurrectedTransferMethods(t *testing.T) {
	banned := regexp.MustCompile(
		`\b(CreateTransfer|UpdateTransfer|UpdateTransferAmount|UpdateTransferDate|` +
			`UpdateTransferStatus|DeleteTransfer|TransferCash|DepositFromAccount|` +
			`TransferCashBetweenInvestments|UpdateTransferCash|GetTransferPair|` +
			`RecreateTransferPair|RestoreVoidedTransfer|NewTransferPair|TransferPair)\s*\(`)

	walkProductionGo(t, func(rel, body string) {
		// This package owns transfers, so its own names are not "resurrected".
		if strings.HasPrefix(rel, filepath.Join("internal", "transfer")+string(filepath.Separator)) {
			return
		}
		// scheduled.TransferPort.CreateTransfer is the sanctioned seam for
		// composing a transfer into a schedule's own transaction — it delegates
		// to this package rather than writing legs itself. See
		// internal/scheduled/transfer_port.go for why it cannot be a direct
		// import.
		if rel == filepath.Join("internal", "scheduled", "transfer_port.go") {
			return
		}

		for i, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue // prose about the old API is fine, and there is a lot of it
			}
			if m := banned.FindString(line); m != "" {
				// The port's own call site is the sanctioned seam.
				if strings.Contains(line, "transferPort.CreateTransfer") {
					continue
				}
				t.Errorf("%s:%d reintroduces a deleted transfer method (%q).\n"+
					"Whole-transaction transfers are owned by internal/transfer; "+
					"call transfer.Service instead of writing legs here.",
					rel, i+1, strings.TrimSuffix(m, "("))
			}
		}
	})
}

// TestArch_InvestmentDoesNotImportTransaction pins the severed edge.
//
// internal/investment imported internal/transaction for as long as cash
// transfers lived there, which is what forced the inverted-dependency counterpart
// port and made a shared transfer impossible. Phase 5 severed it. Re-adding the
// import would not fail the build — it would just quietly restore the coupling —
// so it is asserted here.
func TestArch_InvestmentDoesNotImportTransaction(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "investment")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/investment: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(b), `"github.com/haskovec/tmoney/internal/transaction"`) {
			t.Errorf("internal/investment/%s imports internal/transaction.\n"+
				"That edge was severed in phase 5 of specs/design-unified-transfer.md. "+
				"If this package needs something from the other ledger, it belongs in "+
				"internal/transfer — or behind a consumer-declared port taking db.Queryer.",
				e.Name())
		}
	}
}

// TestArch_NoDuplicateTransferErrorTypes pins the error unification. Each of
// these rules has exactly one owner now, so a caller can match it with a single
// errors.As across package boundaries.
func TestArch_NoDuplicateTransferErrorTypes(t *testing.T) {
	// Type declarations that must not exist anywhere.
	gone := []string{
		"type InvalidTransferAmountError struct", // -> transfer.InvalidAmountError
		"type IsClosedError struct",              // -> account.AccountClosedError
		"type InsufficientCashError struct",      // no producer since phase 5
	}

	walkProductionGo(t, func(rel, body string) {
		for _, decl := range gone {
			if strings.Contains(body, decl) {
				t.Errorf("%s reintroduces %q, which phase 6 unified away.\n"+
					"Duplicated error types cannot be matched with one errors.As, "+
					"which is the whole reason they were merged.", rel, decl)
			}
		}
	})

	// NotRegularAccountError must exist in exactly one package.
	var owners []string
	walkProductionGo(t, func(rel, body string) {
		if strings.Contains(body, "type NotRegularAccountError struct") {
			owners = append(owners, rel)
		}
	})
	if len(owners) != 1 {
		t.Errorf("NotRegularAccountError is declared in %d places (%v); want exactly 1 "+
			"(internal/transaction, which still refuses an investment target for a "+
			"transfer LINE inside a split)", len(owners), owners)
	}
}

// TestArch_GuardActuallyFires is the guard's own guard: a regex that never
// matches is worthless, so prove it matches the thing it is meant to catch.
func TestArch_GuardActuallyFires(t *testing.T) {
	banned := regexp.MustCompile(
		`\b(CreateTransfer|UpdateTransfer|TransferCash|DepositFromAccount|` +
			`TransferCashBetweenInvestments|UpdateTransferCash|GetTransferPair|` +
			`NewTransferPair|TransferPair)\s*\(`)

	// NewTransferPair is listed separately in the pattern on purpose: \b does not
	// create a boundary between "w" and "T", so `\bTransferPair\(` alone would
	// never match `NewTransferPair(`. This self-test is what caught that.
	shouldMatch := []string{
		`	pair, err := s.txnSvc.CreateTransfer(from, to, date, amount, memo, cat)`,
		`	res, err := s.Investment.TransferCash(inv, reg, date, amount, "", cat)`,
		`	_, err := svc.Investment.DepositFromAccount(inv, reg, d, a, "", c)`,
		`	x := NewTransferPair(from, to, date, amount)`,
	}
	for _, line := range shouldMatch {
		if !banned.MatchString(line) {
			t.Errorf("guard regex failed to match a resurrected call: %s", line)
		}
	}

	shouldNotMatch := []string{
		`	res, err := s.svc.Create(transfer.Spec{FromAccountID: from})`,
		`	got, err := s.transferSvc.Resolve(legRowID)`,
		`	if _, err := svc.Transfer.Delete(transferID); err != nil {`,
	}
	for _, line := range shouldNotMatch {
		if banned.MatchString(line) {
			t.Errorf("guard regex wrongly flagged a legitimate transfer-owner call: %s", line)
		}
	}
}
