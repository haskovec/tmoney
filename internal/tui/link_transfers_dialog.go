package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// linkTransfersPreviewedMsg carries the FindUnlinked result back to the
// Update loop so the confirm dialog can be built on the main goroutine.
type linkTransfersPreviewedMsg struct {
	result *transferlink.Result
}

// linkTransfersCompletedMsg is dispatched when the link operation has run.
type linkTransfersCompletedMsg struct {
	linked    int
	ambiguous int
	errors    []error
}

// startLinkTransfers kicks off a FindUnlinked scan and returns a message
// that opens the preview dialog.
func (a *App) startLinkTransfers() tea.Cmd {
	return func() tea.Msg {
		svc := app.NewServices(a.db)
		res, err := svc.TransferLink.FindUnlinked(transferlink.DefaultMaxDateDiffDays)
		if err != nil {
			return errMsg{err: fmt.Errorf("scan failed: %w", err)}
		}
		return linkTransfersPreviewedMsg{result: res}
	}
}

// runLinkTransfersExecute applies the linkage to the database. Re-runs
// the scan so we link against current state (the user may have edited
// transactions in between preview and confirm).
func (a *App) runLinkTransfersExecute() tea.Cmd {
	return func() tea.Msg {
		svc := app.NewServices(a.db)
		res, err := svc.TransferLink.FindUnlinked(transferlink.DefaultMaxDateDiffDays)
		if err != nil {
			return errMsg{err: fmt.Errorf("scan failed: %w", err)}
		}
		linked, errs := svc.TransferLink.Link(res.Clean)
		return linkTransfersCompletedMsg{
			linked:    linked,
			ambiguous: len(res.Ambiguous),
			errors:    errs,
		}
	}
}

// buildLinkTransfersDialog renders the preview summary with sample pairs
// and a Confirm button. If the result is empty, the dialog only offers
// a Close action.
func buildLinkTransfersDialog(r *transferlink.Result) *dialog.Dialog {
	d := dialog.NewDialog("Link Transfers")
	d.SetWidth(72)

	d.AddTextField("Scanned", fmt.Sprintf("%d eligible transactions", r.Scanned), "", 0)
	d.AddTextField("Clean pairs", fmt.Sprintf("%d (will be linked)", len(r.Clean)), "", 0)
	d.AddTextField("Ambiguous", fmt.Sprintf("%d (need manual review)", len(r.Ambiguous)), "", 0)

	if len(r.Clean) > 0 {
		preview := candidatePreviewLines(r.Clean, 5)
		d.AddTextField("Sample", preview, "", 0)
	}
	if len(r.Ambiguous) > 0 {
		preview := candidatePreviewLines(r.Ambiguous, 3)
		d.AddTextField("Ambiguous sample", preview, "", 0)
	}

	primary := "Link"
	if len(r.Clean) == 0 {
		primary = "Close"
	}
	d.SetButtons([]dialog.DialogButton{
		{Label: primary, Primary: true},
		{Label: "Cancel"},
	})
	d.SetFocusIndex(len(d.Fields()))
	d.SetVisible(true)
	return d
}

// candidatePreviewLines returns a single-line, semicolon-joined preview
// of up to maxN candidates.
func candidatePreviewLines(cs []*transferlink.Candidate, maxN int) string {
	if len(cs) < maxN {
		maxN = len(cs)
	}
	parts := make([]string, 0, maxN+1)
	for i := 0; i < maxN; i++ {
		c := cs[i]
		parts = append(parts, fmt.Sprintf("%s %s→%s %s",
			c.From.Date.String(), c.FromAccount, c.ToAccount, c.From.Amount.String()))
	}
	if len(cs) > maxN {
		parts = append(parts, fmt.Sprintf("…and %d more", len(cs)-maxN))
	}
	return strings.Join(parts, "; ")
}

// linkTransfersSurface is the Link Transfers preview dialog's state: the dialog
// and the scan result its confirm step re-checks.
type linkTransfersSurface struct {
	modalSurface
	result *transferlink.Result
}

// IsVisible must be declared here rather than promoted from modalSurface — see
// the note on modalSurface.
func (s *linkTransfersSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// showPreview opens the dialog over a completed scan, keeping the result
// because the confirm step re-checks it before linking.
func (s *linkTransfersSurface) showPreview(result *transferlink.Result) {
	*s = linkTransfersSurface{
		modalSurface: modalSurface{dlg: buildLinkTransfersDialog(result)},
		result:       result,
	}
}

// closeLinkTransfersDialog clears the dialog state.
func (a *App) closeLinkTransfersDialog() {
	a.linkTransfers = linkTransfersSurface{}
}

// handleLinkTransfersDialogKey routes keys to the dialog.
func (a *App) handleLinkTransfersDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.linkTransfers.dlg == nil {
		return a, nil
	}
	return a.linkTransfersDialogAction(a.linkTransfers.dlg.HandleKey(msg))
}

// linkTransfersDialogAction dispatches a DialogAction for the link transfers dialog, from either input path.
func (a *App) linkTransfersDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.closeLinkTransfersDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitLinkTransfersDialog()
	}
	return a, nil
}

// submitLinkTransfersDialog runs the link execute command if there are
// clean pairs to link, or simply closes the dialog otherwise.
func (a *App) submitLinkTransfersDialog() (tea.Model, tea.Cmd) {
	if a.linkTransfers.dlg == nil {
		return a, nil
	}
	if a.linkTransfers.result == nil || len(a.linkTransfers.result.Clean) == 0 {
		a.closeLinkTransfersDialog()
		return a, nil
	}
	a.closeLinkTransfersDialog()
	return a, a.runLinkTransfersExecute()
}

// applyLinkTransfersResult reports what the link pass did and reloads the
// views its writes touched. Pairs it could not link are counted, not listed:
// the user re-runs the scan to see them.
func (a *App) applyLinkTransfersResult(msg linkTransfersCompletedMsg) tea.Cmd {
	summary := fmt.Sprintf("Linked %d transfer pairs", msg.linked)
	if msg.ambiguous > 0 {
		summary += fmt.Sprintf(" (%d ambiguous left for review)", msg.ambiguous)
	}
	a.statusbar.AddNotification(summary, widget.NotificationInfo)
	if len(msg.errors) > 0 {
		parts := make([]string, len(msg.errors))
		for i, e := range msg.errors {
			parts[i] = e.Error()
		}
		a.err = fmt.Errorf("link transfers had %d errors:\n%s",
			len(msg.errors), strings.Join(parts, "\n"))
	}
	return a.reloadAfterBulkWrite()
}
