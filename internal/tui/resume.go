package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// Program resume (OSC 7577). A terminal that supports the protocol (Rex, at
// this time) can run a program again after the terminal restarts. The program
// tells it the command to run: View sets tea.View.ProgramResume, and Bubble
// Tea reports it when the terminal set OSC7577_TOKEN, and again each time the
// command changes. The command is
//
//	<tmoney> -f <absolute file> --resume <view>[/<subject>]
//
// so the terminal already holds the screen on display when it closes, and a
// SIGKILL loses nothing. Only the view and its subject come back (an account,
// or a report type); rows, filters, and dialogs do not.
//
// The --resume value is a contract with every later tmoney: after an upgrade,
// a terminal can run the report of an older binary. So the view names are
// fixed text in the view table, never View numbers, and a value that a
// tmoney does not know opens the Dashboard with a toast, never an error.

// resumeSubject is what follows a view's name in a --resume value.
type resumeSubject int

const (
	// resumeNoSubject: the name alone ("scheduled").
	resumeNoSubject resumeSubject = iota
	// resumeAccount: the name and an account ID ("register/<uuid>").
	resumeAccount
	// resumeReport: the name and a report type ("reports/spending").
	resumeReport
)

// resumeRule is a view's part of the program resume. Every view has one, in
// the view table (views.go).
type resumeRule struct {
	// name is the view's name in a --resume value. It is fixed: a terminal
	// can run the report of an older tmoney, so never change or reuse one.
	name    string
	subject resumeSubject
	// current returns the subject on screen: the account's ID, or the report
	// type's name. It returns "" when there is none yet (the first load is
	// in flight, and no account is selected). Nil for resumeNoSubject.
	current func(*App) string
	// open opens the view for a resume that resolveResume checked: for an
	// account view the account exists, and for Reconciliation the session is
	// the account's active one, or nil. A view that does not fit the account
	// opens the view the sidebar opens for it. Nil for the views that open
	// as the start view does: switchView and the view's reload.
	open func(*App, resumeResolvedMsg) tea.Cmd
}

// resumeDashboard is the --resume value of the start view.
const resumeDashboard = "dashboard"

// reportResumeNames are the names of the report types in a --resume value.
// They are fixed, as the view names are.
var reportResumeNames = map[reportType]string{
	reportTypeNetWorth:   "net-worth",
	reportTypeSpending:   "spending",
	reportTypeHoldings:   "holdings",
	reportTypeAllocation: "allocation",
}

// resumeTarget is a parsed --resume value: the screen a resume opens.
type resumeTarget struct {
	view    View
	account types.ID   // the account, for a resumeAccount view
	report  reportType // the report type, for a resumeReport view
}

// errResumeValue reports a --resume value that this tmoney cannot open.
var errResumeValue = errors.New("not a screen that tmoney can resume")

// parseResume parses a --resume value. A report view with no type opens the
// Net Worth report, as the Reports key does.
func parseResume(s string) (resumeTarget, error) {
	name, subject, hasSubject := strings.Cut(s, "/")
	for _, e := range views() {
		if e.resume.name == "" || e.resume.name != name {
			continue
		}
		t := resumeTarget{view: e.id}
		switch e.resume.subject {
		case resumeNoSubject:
			if hasSubject {
				return resumeTarget{}, fmt.Errorf("%q: %w", s, errResumeValue)
			}
		case resumeAccount:
			id, err := types.ParseID(subject)
			if err != nil || id.IsNil() {
				return resumeTarget{}, fmt.Errorf("%q: %w", s, errResumeValue)
			}
			t.account = id
		case resumeReport:
			if !hasSubject {
				return t, nil // reportTypeNetWorth
			}
			found := false
			for rt, n := range reportResumeNames {
				if n == subject {
					t.report, found = rt, true
				}
			}
			if !found {
				return resumeTarget{}, fmt.Errorf("%q: %w", s, errResumeValue)
			}
		}
		return t, nil
	}
	return resumeTarget{}, fmt.Errorf("%q: %w", s, errResumeValue)
}

// String returns the --resume value of t.
func (t resumeTarget) String() string {
	e, ok := viewFor(t.view)
	if !ok || e.resume.name == "" {
		return resumeDashboard
	}
	switch e.resume.subject {
	case resumeAccount:
		return e.resume.name + "/" + t.account.String()
	case resumeReport:
		return e.resume.name + "/" + reportResumeNames[t.report]
	}
	return e.resume.name
}

// resumeValue returns the --resume value of the screen on display. While an
// account view has no account yet, it is the Dashboard's.
func (a *App) resumeValue() string {
	e, ok := viewFor(a.currentView)
	if !ok || e.resume.name == "" {
		return resumeDashboard
	}
	if e.resume.subject == resumeNoSubject || e.resume.current == nil {
		return e.resume.name
	}
	subject := e.resume.current(a)
	if subject == "" {
		return resumeDashboard
	}
	return e.resume.name + "/" + subject
}

// accountSubject is the current func of the account views: the ID of acct,
// the account the view has loaded, or while it loads, the account selected in
// the sidebar.
func (a *App) accountSubject(acct *account.Account) string {
	id := types.NilID
	if acct != nil {
		id = acct.ID
	} else if a.sidebar != nil {
		id = a.sidebar.SelectedAccountID()
	}
	if id.IsNil() {
		return ""
	}
	return id.String()
}

// programResume returns the report for tea.View.ProgramResume: the command
// that opens this file on this screen. It is nil when the user quits, so the
// terminal does not bring tmoney back (a hangup does not set quitting), and
// when there is no program to name (see resumeExecutable).
func (a *App) programResume() *tea.ProgramResume {
	if a.quitting || a.resumeExe == "" || a.db == nil {
		return nil
	}
	file, err := filepath.Abs(a.db.Path())
	if err != nil {
		return nil
	}
	return &tea.ProgramResume{Args: []string{a.resumeExe, "-f", file, "--resume", a.resumeValue()}}
}

// resumeExecutable returns the absolute path of the running binary, for the
// resume report, or "" when there is none to report. The path is the one that
// ran, not a bare "tmoney" from PATH: a development build that migrated the
// file must not come back as an installed tmoney that refuses the newer
// schema. A binary of the go tool is not reported: the go tool deletes it,
// or can delete it at any time (see reportableExecutable).
func resumeExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return reportableExecutable(exe)
}

// goCacheExecutableDir is the folder of an executable in the go build cache:
// <GOCACHE>/<xx>/<64 hex digits>-d/<name>.
var goCacheExecutableDir = regexp.MustCompile(`^[0-9a-f]{64}-d$`)

// reportableExecutable returns exe, or "" when exe is not absolute or the go
// tool built it:
//   - `go test`, and the first `go run` of a build, link the binary in a
//     go-build<N> work folder, which the go tool deletes when the program
//     exits.
//   - A later `go run` of the same build runs the copy that the go tool keeps
//     in the build cache (GOCACHE, by default a go-build folder). `go clean
//     -cache` and the cache's trim delete it.
func reportableExecutable(exe string) string {
	if !filepath.IsAbs(exe) {
		return ""
	}
	if goCacheExecutableDir.MatchString(filepath.Base(filepath.Dir(exe))) {
		return ""
	}
	for dir := filepath.Dir(exe); ; dir = filepath.Dir(dir) {
		if strings.HasPrefix(filepath.Base(dir), "go-build") {
			return ""
		}
		if parent := filepath.Dir(dir); parent == dir {
			return exe
		}
	}
}

// setResume takes the --resume value of the start. Init starts the resume
// (see startResume). A value that does not parse shows a toast; Init
// schedules its clear.
func (a *App) setResume(s string) {
	if s == "" {
		return
	}
	t, err := parseResume(s)
	if err != nil {
		a.statusbar.SetToast(resumeFailedText, widget.NotificationAlert)
		return
	}
	a.pendingResume = &t
}

// startResume returns, for Init, the commands that open the pending resume,
// or false when there is none. They are the auto-post of the open, and after
// it the sidebar's first load, whose message carries the resume (see the
// sidebarLoadedMsg arm):
//   - After the auto-post: on a normal start the auto-post's result reloads
//     the Dashboard, but it does not reload a resumed view, which would then
//     miss the posted rows.
//   - With the sidebar: the sidebar can then select the account.
//   - In that one message: a first load that fails, or that a File > Open
//     makes stale, drops the resume with it, so no later load of the sidebar
//     starts it on another file.
func (a *App) startResume() (tea.Cmd, bool) {
	t := a.pendingResume
	if t == nil {
		return nil, false
	}
	a.pendingResume = nil
	target := *t
	load := a.loadSidebarData()
	withResume := func() tea.Msg {
		msg := load()
		if loaded, ok := msg.(sidebarLoadedMsg); ok {
			loaded.resume = &target
			return loaded
		}
		return msg
	}
	return tea.Sequence(a.autoPostOnFileOpen(), withResume), true
}

// resumeFailedText is the toast of a resume that falls back to the Dashboard.
const resumeFailedText = "Could not resume the last screen; this is the Dashboard"

// resumeResolvedMsg is the result of resolveResume.
type resumeResolvedMsg struct {
	target resumeTarget
	// account is the target's account, for an account view.
	account *account.Account
	// session is the account's active reconciliation session, or nil, for
	// Reconciliation.
	session *reconciliation.Session
	// err is why the resume cannot open the target: the account is gone, or
	// a read failed. The Dashboard stays, with a toast.
	err error
	fileOf
}

// resolveResume reads what opening t needs: its account, which must still
// exist, and for Reconciliation the account's active session.
func (a *App) resolveResume(t resumeTarget) tea.Cmd {
	svc := a.services
	file := a.fileOfOpen()
	e, _ := viewFor(t.view)
	return func() tea.Msg {
		msg := resumeResolvedMsg{target: t, fileOf: file}
		if e.resume.subject != resumeAccount {
			return msg
		}
		if svc.Account == nil {
			msg.err = errors.New("no account service")
			return msg
		}
		acct, err := svc.Account.GetByID(t.account)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.account = acct
		if t.view == ViewReconciliation && svc.Reconciliation != nil {
			session, err := svc.Reconciliation.GetActiveSession(acct.ID)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.session = session
		}
		return msg
	}
}

// applyResume opens the screen of a resolved resume. It does nothing when the
// user has already left the start view: what they opened wins.
func (a *App) applyResume(msg resumeResolvedMsg) tea.Cmd {
	if a.currentView != ViewDashboard {
		return nil
	}
	if msg.err != nil {
		a.statusbar.SetToast(resumeFailedText, widget.NotificationAlert)
		return widget.ClearToastCmd()
	}
	e, ok := viewFor(msg.target.view)
	if !ok || msg.target.view == ViewDashboard {
		return nil
	}
	if msg.account != nil {
		a.sidebar.SelectAccount(msg.account.ID)
	}
	if e.resume.open != nil {
		return e.resume.open(a, msg)
	}
	a.switchView(msg.target.view)
	return tea.Batch(e.reload(a)...)
}

// openAccountView opens the view the sidebar opens for acct: the portfolio
// for an investment account, the register otherwise. A resume uses it for an
// account whose view no longer fits it.
func (a *App) openAccountView(acct *account.Account) tea.Cmd {
	if acct.Type.IsInvestmentType() {
		a.portfolio.data = nil
		a.switchView(ViewPortfolio)
		return a.portfolio.load(a.portfolioDeps(), acct.ID)
	}
	a.register.clearForLoad()
	a.switchView(ViewRegister)
	return a.loadRegisterData(acct.ID)
}

// resumeReconciliationText is the toast of a resumed reconciliation: the
// session is saved, its check marks are not.
const resumeReconciliationText = "Reconciliation resumed. Check the cleared items again"

// resumeReconciliation continues the account's active reconciliation session,
// with no item checked. With no session, the account's view opens.
func (a *App) resumeReconciliation(msg resumeResolvedMsg) tea.Cmd {
	if msg.session == nil || msg.account.Type.IsInvestmentType() {
		return a.openAccountView(msg.account)
	}
	a.switchView(ViewReconciliation)
	a.statusbar.SetToast(resumeReconciliationText, widget.NotificationInfo)
	return tea.Batch(a.loadReconciliationData(msg.session, msg.account), widget.ClearToastCmd())
}
