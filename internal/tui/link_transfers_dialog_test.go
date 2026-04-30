package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/types"
)

func makeCandidate(fromAcct, toAcct, dateStr, amount string) *transferlink.Candidate {
	d := types.MustParseDate(dateStr)
	from := transaction.NewTransaction(types.NewID(), d, types.MustNewMoney(amount).Neg())
	to := transaction.NewTransaction(types.NewID(), d, types.MustNewMoney(amount))
	return &transferlink.Candidate{
		From:        from,
		To:          to,
		FromAccount: fromAcct,
		ToAccount:   toAcct,
	}
}

func TestBuildLinkTransfersDialog(t *testing.T) {
	t.Run("primary button is Link when there are clean pairs", func(t *testing.T) {
		r := &transferlink.Result{
			Clean:   []*transferlink.Candidate{makeCandidate("Checking", "Savings", "2024-01-10", "100.00")},
			Scanned: 4,
		}
		d := buildLinkTransfersDialog(r)
		if d.Title() != "Link Transfers" {
			t.Errorf("title = %q, want 'Link Transfers'", d.Title())
		}
		var primary string
		for _, b := range d.Buttons() {
			if b.Primary {
				primary = b.Label
			}
		}
		if primary != "Link" {
			t.Errorf("primary button = %q, want 'Link'", primary)
		}
	})

	t.Run("primary button is Close when nothing to link", func(t *testing.T) {
		r := &transferlink.Result{Scanned: 0}
		d := buildLinkTransfersDialog(r)
		var primary string
		for _, b := range d.Buttons() {
			if b.Primary {
				primary = b.Label
			}
		}
		if primary != "Close" {
			t.Errorf("primary button = %q, want 'Close'", primary)
		}
	})

	t.Run("focus defaults to primary button", func(t *testing.T) {
		r := &transferlink.Result{Scanned: 0}
		d := buildLinkTransfersDialog(r)
		want := len(d.Fields()) + 1
		if d.FocusIndex() != want {
			t.Errorf("focusIndex = %d, want %d", d.FocusIndex(), want)
		}
	})

	t.Run("ambiguous sample appears when ambiguous pairs exist", func(t *testing.T) {
		r := &transferlink.Result{
			Ambiguous: []*transferlink.Candidate{
				makeCandidate("Checking", "Savings", "2024-02-01", "50.00"),
			},
			Scanned: 2,
		}
		d := buildLinkTransfersDialog(r)
		var found bool
		for _, f := range d.Fields() {
			if f.Label == "Ambiguous sample" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected an Ambiguous sample field when ambiguous candidates are present")
		}
	})
}

func TestCandidatePreviewLines(t *testing.T) {
	t.Run("respects maxN", func(t *testing.T) {
		cs := []*transferlink.Candidate{
			makeCandidate("A", "B", "2024-01-01", "1.00"),
			makeCandidate("A", "B", "2024-01-02", "2.00"),
			makeCandidate("A", "B", "2024-01-03", "3.00"),
		}
		out := candidatePreviewLines(cs, 2)
		if !strings.Contains(out, "and 1 more") {
			t.Errorf("expected 'and 1 more' in %q", out)
		}
	})

	t.Run("no overflow message when all fit", func(t *testing.T) {
		cs := []*transferlink.Candidate{
			makeCandidate("A", "B", "2024-01-01", "1.00"),
		}
		out := candidatePreviewLines(cs, 5)
		if strings.Contains(out, "more") {
			t.Errorf("did not expect overflow message in %q", out)
		}
	})
}
