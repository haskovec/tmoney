package tui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// Every view has a fixed, unique resume name, and the value of every view and
// subject parses back to the same screen.
func TestResume_EveryViewRoundTrips(t *testing.T) {
	seen := map[string]View{}
	for _, e := range views() {
		if e.resume.name == "" {
			t.Errorf("%v has no resume name in allViews (views.go); add one", e.id)
			continue
		}
		if other, dup := seen[e.resume.name]; dup {
			t.Errorf("%v and %v share the resume name %q", e.id, other, e.resume.name)
		}
		seen[e.resume.name] = e.id
		if (e.resume.subject == resumeNoSubject) != (e.resume.current == nil) {
			t.Errorf("%v: a view has a current func exactly when it has a subject", e.id)
		}

		targets := []resumeTarget{{view: e.id}}
		switch e.resume.subject {
		case resumeAccount:
			targets = []resumeTarget{{view: e.id, account: types.NewID()}}
		case resumeReport:
			targets = nil
			for rt := range reportResumeNames {
				targets = append(targets, resumeTarget{view: e.id, report: rt})
			}
		}
		for _, want := range targets {
			got, err := parseResume(want.String())
			if err != nil || got != want {
				t.Errorf("parseResume(%q) = %+v, %v; want %+v", want.String(), got, err, want)
			}
		}
	}
}

// reportTypeConstants returns every reportType constant in reports_view.go,
// in declaration order, as viewConstants does for View.
func reportTypeConstants(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("reports_view.go")
	if err != nil {
		t.Fatalf("read reports_view.go: %v", err)
	}
	names, err := iotaConstantNames(src, "reportType")
	if err != nil {
		t.Fatalf("enumerate reportType constants: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("found no reportType constants in reports_view.go, so the test would pass vacuously")
	}
	return names
}

// Every report type has a resume name, and no two share one.
func TestResume_EveryReportTypeHasAName(t *testing.T) {
	consts := reportTypeConstants(t)
	seen := map[string]bool{}
	for i, name := range consts {
		n, ok := reportResumeNames[reportType(i)]
		if !ok || n == "" {
			t.Errorf("%s has no name in reportResumeNames (resume.go); add one", name)
		}
		if seen[n] {
			t.Errorf("two report types share the resume name %q", n)
		}
		seen[n] = true
	}
	if len(reportResumeNames) != len(consts) {
		t.Errorf("reportResumeNames has %d names for %d report types", len(reportResumeNames), len(consts))
	}
}

// The --resume values are a contract with every later tmoney: a terminal can
// run the report of an older binary after an upgrade. These are the published
// names. Add a name for a new view or report; never change or remove one.
var publishedResumeValues = map[string]resumeTarget{
	"dashboard":          {view: ViewDashboard},
	"scheduled":          {view: ViewScheduled},
	"securities":         {view: ViewSecurities},
	"prices":             {view: ViewPrices},
	"corporate-actions":  {view: ViewCorporateActions},
	"reports":            {view: ViewReports, report: reportTypeNetWorth},
	"reports/net-worth":  {view: ViewReports, report: reportTypeNetWorth},
	"reports/spending":   {view: ViewReports, report: reportTypeSpending},
	"reports/holdings":   {view: ViewReports, report: reportTypeHoldings},
	"reports/allocation": {view: ViewReports, report: reportTypeAllocation},
}

// publishedAccountViews are the published names of the account views.
var publishedAccountViews = map[string]View{
	"register":            ViewRegister,
	"reconciliation":      ViewReconciliation,
	"investment-register": ViewInvestmentRegister,
	"portfolio":           ViewPortfolio,
	"amortization":        ViewAmortization,
}

func TestParseResume(t *testing.T) {
	id := types.NewID()
	ok := map[string]resumeTarget{}
	for s, want := range publishedResumeValues {
		ok[s] = want
	}
	for name, v := range publishedAccountViews {
		ok[name+"/"+id.String()] = resumeTarget{view: v, account: id}
	}
	for s, want := range ok {
		if got, err := parseResume(s); err != nil || got != want {
			t.Errorf("parseResume(%q) = %+v, %v; want %+v", s, got, err, want)
		}
	}
	bad := []string{
		"", "nope", "Register/" + id.String(), "register", "register/", "register/not-an-id",
		"register/" + types.NilID.String(), "scheduled/x", "reports/nope", "dashboard/",
	}
	for _, s := range bad {
		if got, err := parseResume(s); !errors.Is(err, errResumeValue) {
			t.Errorf("parseResume(%q) = %+v, %v; want errResumeValue", s, got, err)
		}
	}
}

func TestReportableExecutable(t *testing.T) {
	// A root that is absolute on every OS: "/" on Unix, "C:\" on Windows.
	root, err := filepath.Abs(string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	abs := func(p ...string) string { return filepath.Join(append([]string{root}, p...)...) }
	hash := strings.Repeat("0123456789abcdef", 4) // 64 hex digits

	tests := map[string]string{
		abs("usr", "local", "bin", "tmoney"):                     abs("usr", "local", "bin", "tmoney"),
		abs("Users", "me", "src", "tmoney", "tmoney"):            abs("Users", "me", "src", "tmoney", "tmoney"),
		abs("var", "folders", "x", "T", "TestFoo1", "001", "tm"): abs("var", "folders", "x", "T", "TestFoo1", "001", "tm"),
		// go test, and the first go run of a build: the work folder.
		abs("var", "folders", "x", "T", "go-build123", "b001", "exe", "tmoney"): "",
		abs("tmp", "go-build456", "b001", "cli.test"):                           "",
		// A later go run: the copy in the build cache, also outside a
		// go-build folder.
		abs("Users", "me", "Library", "Caches", "go-build", "26", hash+"-d", "tmoney"): "",
		abs("home", "me", "gocache", "26", hash+"-d", "tmoney"):                        "",
		"tmoney":                     "",
		filepath.Join(".", "tmoney"): "",
	}
	for exe, want := range tests {
		if got := reportableExecutable(exe); got != want {
			t.Errorf("reportableExecutable(%q) = %q, want %q", exe, got, want)
		}
	}
}

// resumeTestApp is an App on a fresh database, with a program to report.
// Toasts clear at once (see runKeepingToast).
func resumeTestApp(t *testing.T) *App {
	t.Helper()
	old := widget.ToastDuration
	widget.ToastDuration = 0
	t.Cleanup(func() { widget.ToastDuration = old })
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40
	a.resumeExe = "/usr/local/bin/tmoney"
	return a
}

// newResumeAccount creates an account of type typ in a's database.
func newResumeAccount(t *testing.T, a *App, name string, typ account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, typ, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	return acct
}

func TestProgramResume(t *testing.T) {
	a := resumeTestApp(t)
	file, err := filepath.Abs(a.db.Path())
	if err != nil {
		t.Fatal(err)
	}
	args := func() []string {
		t.Helper()
		pr := a.View().ProgramResume
		if pr == nil {
			return nil
		}
		return pr.Args
	}
	want := func(value string) []string {
		return []string{"/usr/local/bin/tmoney", "-f", file, "--resume", value}
	}
	equal := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	if got := args(); !equal(got, want("dashboard")) {
		t.Errorf("Dashboard: args = %q, want %q", got, want("dashboard"))
	}

	// An account view whose load is in flight names the sidebar's account.
	acct := newResumeAccount(t, a, "Checking", account.TypeChecking)
	a.sidebar.SelectAccount(acct.ID)
	a.register.clearForLoad()
	a.switchView(ViewRegister)
	if got := args(); !equal(got, want("register/"+acct.ID.String())) {
		t.Errorf("Register while loading: args = %q", got)
	}
	runCmd(t, a, a.loadRegisterData(acct.ID), 1)
	if got := args(); !equal(got, want("register/"+acct.ID.String())) {
		t.Errorf("Register: args = %q", got)
	}

	// An account view with no account at all reports the Dashboard.
	a.sidebar.SelectAccount(types.NilID)
	a.amortization.data = nil
	a.switchView(ViewAmortization)
	if got := args(); !equal(got, want("dashboard")) {
		t.Errorf("Amortization with no account: args = %q", got)
	}

	a.switchView(ViewScheduled)
	if got := args(); !equal(got, want("scheduled")) {
		t.Errorf("Scheduled: args = %q", got)
	}

	a.switchView(ViewReports)
	a.reports.data = &reportsViewData{rtype: reportTypeSpending}
	if got := args(); !equal(got, want("reports/spending")) {
		t.Errorf("Reports: args = %q", got)
	}

	// The user quits: no report, so the terminal does not bring tmoney back.
	a.quitting = true
	if got := a.View().ProgramResume; got != nil {
		t.Errorf("after a quit: ProgramResume = %+v, want nil", got)
	}
	a.quitting = false

	// No program to name: no report.
	a.resumeExe = ""
	if got := a.View().ProgramResume; got != nil {
		t.Errorf("with no program: ProgramResume = %+v, want nil", got)
	}
}

// runKeepingToast is runCmd, but it drops the clear of a toast, so that the
// test can read the toast after the commands ran. It also runs a tea.Sequence
// (whose message type is not exported) in order.
func runKeepingToast(t *testing.T, a *App, cmd tea.Cmd, depth int) {
	t.Helper()
	runCmds(t, a, cmd, depth, false)
}

// runCmds is runKeepingToast. With failures, it gives Update the errMsg of a
// failed command too, where runKeepingToast fails the test.
func runCmds(t *testing.T, a *App, cmd tea.Cmd, depth int, failures bool) {
	t.Helper()
	if cmd == nil || depth == 0 {
		return
	}
	msg := cmd()
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		for i := range v.Len() {
			runCmds(t, a, v.Index(i).Interface().(tea.Cmd), depth, failures)
		}
		return
	}
	if e, ok := msg.(errMsg); ok && !failures {
		t.Fatalf("command failed: %v", e.err)
	}
	switch msg.(type) {
	case nil, widget.ToastClearMsg:
	default:
		_, next := a.Update(msg)
		runCmds(t, a, next, depth-1, failures)
	}
}

// resumeTo starts a resume to value on a, as Run and Init do, and runs the
// commands of the start that open it.
func resumeTo(t *testing.T, a *App, value string) {
	t.Helper()
	a.setResume(value)
	cmd, ok := a.startResume()
	if !ok {
		t.Fatalf("setResume(%q) left no resume to start", value)
	}
	runKeepingToast(t, a, cmd, 4)
}

func TestResume_OpensTheScreen(t *testing.T) {
	a := resumeTestApp(t)
	ids := map[string]types.ID{
		"checking":  newResumeAccount(t, a, "Checking", account.TypeChecking).ID,
		"loan":      newResumeAccount(t, a, "Car Loan", account.TypeLoan).ID,
		"brokerage": newResumeAccount(t, a, "Brokerage", account.TypeInvestment).ID,
	}
	// withID turns "<view>/<key>" into "<view>/<uuid>".
	withID := func(s string) string {
		if name, key, ok := strings.Cut(s, "/"); ok {
			if id, isAccount := ids[key]; isAccount {
				return name + "/" + id.String()
			}
		}
		return s
	}

	// value is the --resume value; want is the screen it opens, as its own
	// --resume value, so it also names the account that the view loaded.
	tests := []struct{ value, want string }{
		{"register/checking", "register/checking"},
		{"register/brokerage", "portfolio/brokerage"},
		{"portfolio/brokerage", "portfolio/brokerage"},
		{"portfolio/checking", "register/checking"},
		{"investment-register/brokerage", "investment-register/brokerage"},
		{"investment-register/checking", "register/checking"},
		{"amortization/loan", "amortization/loan"},
		{"amortization/checking", "register/checking"},
		{"reconciliation/checking", "register/checking"}, // no active session
		{"reports/holdings", "reports/holdings"},
		{"scheduled", "scheduled"},
		{"securities", "securities"},
		{"prices", "prices"},
		{"corporate-actions", "corporate-actions"},
		{"dashboard", "dashboard"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			a.switchView(ViewDashboard)
			a.sidebar.SelectAccount(types.NilID)
			a.statusbar.ClearToast()
			resumeTo(t, a, withID(tt.value))
			if got, want := a.resumeValue(), withID(tt.want); got != want {
				t.Errorf("the screen is %q, want %q", got, want)
			}
			if toast := a.statusbar.Toast(); toast != nil {
				t.Errorf("a resume that worked shows the toast %q", toast.Text)
			}
		})
	}
}

// A resume into Reconciliation continues the account's active session, with
// a toast that the check marks are gone.
func TestResume_ContinuesTheReconciliation(t *testing.T) {
	a := resumeTestApp(t)
	checking := newResumeAccount(t, a, "Checking", account.TypeChecking)
	session, err := a.services.Reconciliation.StartReconciliation(checking.ID, types.Today(), types.ZeroMoney)
	if err != nil {
		t.Fatal(err)
	}

	resumeTo(t, a, "reconciliation/"+checking.ID.String())
	if a.currentView != ViewReconciliation {
		t.Fatalf("view = %v, want Reconciliation", a.currentView)
	}
	if d := a.reconciliation.data; d == nil || d.session == nil || d.session.ID != session.ID {
		t.Fatalf("the reconciliation does not show the active session %v", session.ID)
	}
	if toast := a.statusbar.Toast(); toast == nil || toast.Text != resumeReconciliationText {
		t.Errorf("toast = %+v, want %q", toast, resumeReconciliationText)
	}
	if got, want := a.resumeValue(), "reconciliation/"+checking.ID.String(); got != want {
		t.Errorf("resumeValue = %q, want %q", got, want)
	}
}

// A resume that cannot open its screen leaves the Dashboard with a toast: a
// deleted account, or a value that this tmoney does not know.
func TestResume_FallsBackToTheDashboard(t *testing.T) {
	a := resumeTestApp(t)
	resumeTo(t, a, "register/"+types.NewID().String())
	if a.currentView != ViewDashboard {
		t.Errorf("view = %v after a resume to a missing account, want Dashboard", a.currentView)
	}
	if toast := a.statusbar.Toast(); toast == nil || toast.Text != resumeFailedText || toast.Level != widget.NotificationAlert {
		t.Errorf("toast = %+v, want the alert %q", toast, resumeFailedText)
	}

	b := resumeTestApp(t)
	b.setResume("a-view-from-the-future")
	if b.pendingResume != nil {
		t.Error("an unknown value is pending")
	}
	if toast := b.statusbar.Toast(); toast == nil || toast.Text != resumeFailedText {
		t.Errorf("toast = %+v, want %q", toast, resumeFailedText)
	}
	// Init schedules the clear of the toast (ToastDuration is 0 here); an
	// App with no toast does not.
	initClearsToast := func(a *App) bool {
		batch, ok := a.Init()().(tea.BatchMsg)
		if !ok {
			return false
		}
		for _, c := range batch {
			if c == nil {
				continue
			}
			if _, ok := c().(widget.ToastClearMsg); ok {
				return true
			}
		}
		return false
	}
	if !initClearsToast(b) {
		t.Error("Init does not schedule the clear of the toast")
	}
	if initClearsToast(resumeTestApp(t)) {
		t.Error("Init schedules a toast clear with no toast")
	}
}

// What the user opened before the resume came back wins.
func TestResume_DoesNotMoveAUserWhoLeftTheDashboard(t *testing.T) {
	a := resumeTestApp(t)
	checking := newResumeAccount(t, a, "Checking", account.TypeChecking)
	a.setResume("register/" + checking.ID.String())
	t1 := *a.pendingResume
	a.pendingResume = nil
	resolved := a.resolveResume(t1)()
	a.switchView(ViewScheduled)
	_, cmd := a.Update(resolved)
	if a.currentView != ViewScheduled {
		t.Errorf("view = %v, want Scheduled (the user's choice)", a.currentView)
	}
	if cmd != nil {
		t.Errorf("applyResume returned a command after the user moved on")
	}
}

// resumeProbe runs an App in a real tea.Program, whose commands run in their
// own goroutines, and stops the program when done reports true.
type resumeProbe struct {
	*App
	autoPosted bool
	done       func(*resumeProbe) bool
}

func (p *resumeProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(autoPostCompletedMsg); ok {
		p.autoPosted = true
	}
	_, cmd := p.App.Update(msg)
	if p.done(p) {
		return p, tea.Quit
	}
	return p, cmd
}

// A resume opens its view after the auto-post of the open, so the view shows
// the rows that the auto-post wrote. The auto-post's result reloads only the
// Dashboard; a view that loaded before the auto-post ended stayed without the
// rows. The race needs real goroutines, so this runs a real program.
func TestResume_ViewShowsTheAutoPostedRows(t *testing.T) {
	a := resumeTestApp(t)
	checking := newResumeAccount(t, a, "Checking", account.TypeChecking)
	st := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-12.34"))
	st.AutoPost = true
	if err := a.services.Scheduled.Create(st); err != nil {
		t.Fatal(err)
	}
	a.setResume("register/" + checking.ID.String())

	probe := &resumeProbe{App: a, done: func(p *resumeProbe) bool {
		return p.autoPosted && p.currentView == ViewRegister && p.register.data != nil
	}}
	prog := tea.NewProgram(probe, tea.WithInput(nil), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler())
	errc := make(chan error, 1)
	go func() {
		_, err := prog.Run()
		errc <- err
	}()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(15 * time.Second):
		prog.Kill()
		<-errc
		t.Fatalf("the resume did not open the register within 15s (view %v)", a.currentView)
	}

	if n := len(a.register.data.transactions); n != 1 {
		t.Errorf("the resumed register shows %d rows, want the 1 auto-posted row", n)
	}
}

// Only the start's first sidebar load carries the resume. When that load is
// dropped, a later load does not start the resume, on this file or another.
func TestResume_DroppedFirstLoadDropsTheResume(t *testing.T) {
	a := resumeTestApp(t)
	a.setResume("scheduled")
	cmd, ok := a.startResume()
	if !ok {
		t.Fatal("no resume to start")
	}
	// File > Open before the first load comes back: the load is stale.
	a.switchDatabase(dbtest.New(t))
	runKeepingToast(t, a, cmd, 4)
	runKeepingToast(t, a, a.loadSidebarData(), 4)
	if a.currentView != ViewDashboard {
		t.Errorf("view = %v after a dropped first load, want Dashboard", a.currentView)
	}
	if a.pendingResume != nil {
		t.Error("the resume is still pending")
	}
}

// Esc in a resumed reconciliation cancels it and shows the account's register,
// loaded: the resume never loaded the register first.
func TestResume_CancelledReconciliationLoadsTheRegister(t *testing.T) {
	a := resumeTestApp(t)
	checking := newResumeAccount(t, a, "Checking", account.TypeChecking)
	if _, err := a.services.Reconciliation.StartReconciliation(checking.ID, types.Today(), types.ZeroMoney); err != nil {
		t.Fatal(err)
	}
	resumeTo(t, a, "reconciliation/"+checking.ID.String())
	if a.currentView != ViewReconciliation {
		t.Fatalf("view = %v, want Reconciliation", a.currentView)
	}

	_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	runKeepingToast(t, a, cmd, 4)
	if a.currentView != ViewRegister {
		t.Fatalf("view = %v after Esc, want Register", a.currentView)
	}
	if d := a.register.data; d == nil || d.account == nil || d.account.ID != checking.ID {
		t.Errorf("the register did not load the account after the cancel")
	}
}

// While a resume opens, the report keeps the screen of the resume: the
// Dashboard is on display only until the auto-post and the sidebar's first
// load end, and a second restart in that window must not lose the screen.
func TestResume_ReportsItsScreenWhileItOpens(t *testing.T) {
	lastArg := func(a *App) string {
		t.Helper()
		pr := a.View().ProgramResume
		if pr == nil || len(pr.Args) == 0 {
			t.Fatal("no resume report")
		}
		return pr.Args[len(pr.Args)-1]
	}

	t.Run("until it opens", func(t *testing.T) {
		a := resumeTestApp(t)
		a.setResume("scheduled")
		cmd, ok := a.startResume()
		if !ok {
			t.Fatal("no resume to start")
		}
		if got := lastArg(a); got != "scheduled" {
			t.Errorf("while the resume opens the report is %q, want scheduled", got)
		}
		runKeepingToast(t, a, cmd, 4)
		if a.currentView != ViewScheduled || lastArg(a) != "scheduled" {
			t.Errorf("after the resume: view %v, report %q; want Scheduled", a.currentView, lastArg(a))
		}
	})
	t.Run("until it fails", func(t *testing.T) {
		a := resumeTestApp(t)
		missing := "register/" + types.NewID().String()
		a.setResume(missing)
		cmd, _ := a.startResume()
		if got := lastArg(a); got != missing {
			t.Errorf("while the resume opens the report is %q, want %q", got, missing)
		}
		runKeepingToast(t, a, cmd, 4)
		if got := lastArg(a); got != "dashboard" {
			t.Errorf("after a failed resume the report is %q, want dashboard", got)
		}
	})
	t.Run("until the user leaves the Dashboard", func(t *testing.T) {
		a := resumeTestApp(t)
		a.setResume("scheduled")
		if _, ok := a.startResume(); !ok {
			t.Fatal("no resume to start")
		}
		a.switchView(ViewPrices)
		if got := lastArg(a); got != "prices" {
			t.Errorf("after the user opened Prices the report is %q, want prices", got)
		}
	})
	t.Run("until the file changes", func(t *testing.T) {
		a := resumeTestApp(t)
		a.setResume("scheduled")
		if _, ok := a.startResume(); !ok {
			t.Fatal("no resume to start")
		}
		a.switchDatabase(dbtest.New(t))
		if got := lastArg(a); got != "dashboard" {
			t.Errorf("after File > Open the report is %q, want dashboard", got)
		}
	})
	t.Run("until its first load fails", func(t *testing.T) {
		a := resumeTestApp(t)
		a.setResume("scheduled")
		cmd, ok := a.startResume()
		if !ok {
			t.Fatal("no resume to start")
		}
		if err := a.db.Close(); err != nil { // every read of the start fails
			t.Fatal(err)
		}
		runCmds(t, a, cmd, 4, true)
		if a.err == nil {
			t.Error("the failed first load shows no error")
		}
		if a.currentView != ViewDashboard || lastArg(a) != "dashboard" {
			t.Errorf("after a failed first load: view %v, report %q; want the Dashboard", a.currentView, lastArg(a))
		}
	})
	t.Run("until a first load that has no accounts to load", func(t *testing.T) {
		a := resumeTestApp(t)
		a.services.Account = nil // the load returns nothing
		a.setResume("scheduled")
		cmd, ok := a.startResume()
		if !ok {
			t.Fatal("no resume to start")
		}
		runKeepingToast(t, a, cmd, 4)
		if got := lastArg(a); got != "dashboard" {
			t.Errorf("after a first load with no result the report is %q, want dashboard", got)
		}
	})
}
