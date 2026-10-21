package widget

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestNewStatusBar(t *testing.T) {
	sb := NewStatusBar()

	if sb.Context() != "" {
		t.Errorf("Context() = %q, want empty string", sb.Context())
	}
	if sb.KeyHints() != "" {
		t.Errorf("KeyHints() = %q, want empty string", sb.KeyHints())
	}
	if len(sb.Notifications()) != 0 {
		t.Errorf("Notifications() length = %d, want 0", len(sb.Notifications()))
	}
}

func TestStatusBar_SetContext(t *testing.T) {
	sb := NewStatusBar()

	sb.SetContext("Dashboard")
	if sb.Context() != "Dashboard" {
		t.Errorf("Context() = %q, want %q", sb.Context(), "Dashboard")
	}

	sb.SetContext("Checking")
	if sb.Context() != "Checking" {
		t.Errorf("Context() = %q, want %q", sb.Context(), "Checking")
	}

	sb.SetContext("")
	if sb.Context() != "" {
		t.Errorf("Context() = %q, want empty string", sb.Context())
	}
}

func TestStatusBar_SetKeyHints(t *testing.T) {
	sb := NewStatusBar()

	hints := "↑↓ navigate  enter select  ? help"
	sb.SetKeyHints(hints)
	if sb.KeyHints() != hints {
		t.Errorf("KeyHints() = %q, want %q", sb.KeyHints(), hints)
	}
}

func TestStatusBar_AddNotification(t *testing.T) {
	sb := NewStatusBar()

	sb.AddNotification("2 scheduled due", NotificationAlert)
	notifications := sb.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("Notifications() length = %d, want 1", len(notifications))
	}
	if notifications[0].Text != "2 scheduled due" {
		t.Errorf("notification text = %q, want %q", notifications[0].Text, "2 scheduled due")
	}
	if notifications[0].Level != NotificationAlert {
		t.Errorf("notification level = %d, want %d", notifications[0].Level, NotificationAlert)
	}
}

func TestStatusBar_AddNotification_Info(t *testing.T) {
	sb := NewStatusBar()

	sb.AddNotification("File saved", NotificationInfo)
	notifications := sb.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("Notifications() length = %d, want 1", len(notifications))
	}
	if notifications[0].Level != NotificationInfo {
		t.Errorf("notification level = %d, want %d", notifications[0].Level, NotificationInfo)
	}
}

func TestStatusBar_AddNotification_MaxLimit(t *testing.T) {
	sb := NewStatusBar()

	// Add more than maxNotifications
	for i := range maxNotifications + 2 {
		sb.AddNotification(strings.Repeat("x", i+1), NotificationInfo)
	}

	notifications := sb.Notifications()
	if len(notifications) != maxNotifications {
		t.Errorf("Notifications() length = %d, want %d", len(notifications), maxNotifications)
	}

	// The oldest notifications should have been dropped
	// Last notification should be the most recently added
	last := notifications[len(notifications)-1]
	expected := strings.Repeat("x", maxNotifications+2)
	if last.Text != expected {
		t.Errorf("last notification text = %q, want %q", last.Text, expected)
	}
}

func TestStatusBar_ClearNotifications(t *testing.T) {
	sb := NewStatusBar()

	sb.AddNotification("test1", NotificationInfo)
	sb.AddNotification("test2", NotificationAlert)
	sb.ClearNotifications()

	if len(sb.Notifications()) != 0 {
		t.Errorf("Notifications() length = %d, want 0 after clear", len(sb.Notifications()))
	}
}

func TestStatusBar_Render_Empty(t *testing.T) {
	sb := NewStatusBar()
	styles := NewStyles()
	styles.Resize(80, 24)

	result := sb.Render(styles, 80)
	if result == "" {
		t.Error("Render() should not return empty string for width > 0")
	}
}

func TestStatusBar_Render_ZeroWidth(t *testing.T) {
	sb := NewStatusBar()
	styles := NewStyles()

	result := sb.Render(styles, 0)
	if result != "" {
		t.Errorf("Render() = %q, want empty string for zero width", result)
	}
}

func TestStatusBar_Render_NegativeWidth(t *testing.T) {
	sb := NewStatusBar()
	styles := NewStyles()

	result := sb.Render(styles, -1)
	if result != "" {
		t.Errorf("Render() = %q, want empty string for negative width", result)
	}
}

func TestStatusBar_Render_WithContext(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Dashboard")
	styles := NewStyles()
	styles.Resize(80, 24)

	result := sb.Render(styles, 80)
	stripped := StripAnsi(result)
	if !strings.Contains(stripped, "Dashboard") {
		t.Errorf("Render() should contain context 'Dashboard', got: %q", stripped)
	}
}

func TestStatusBar_Render_WithKeyHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetKeyHints("ctrl+q quit")
	styles := NewStyles()
	styles.Resize(80, 24)

	result := sb.Render(styles, 80)
	stripped := StripAnsi(result)
	if !strings.Contains(stripped, "ctrl+q quit") {
		t.Errorf("Render() should contain key hints, got: %q", stripped)
	}
}

func TestStatusBar_Render_WithNotification(t *testing.T) {
	sb := NewStatusBar()
	sb.AddNotification("2 scheduled due", NotificationAlert)
	styles := NewStyles()
	styles.Resize(100, 24)

	result := sb.Render(styles, 100)
	stripped := StripAnsi(result)
	if !strings.Contains(stripped, "2 scheduled due") {
		t.Errorf("Render() should contain notification text, got: %q", stripped)
	}
}

func TestStatusBar_Render_AllSections(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Register")
	sb.SetKeyHints("n new  d delete  ? help")
	sb.AddNotification("1 due", NotificationAlert)
	styles := NewStyles()
	styles.Resize(120, 24)

	result := sb.Render(styles, 120)
	stripped := StripAnsi(result)

	if !strings.Contains(stripped, "Register") {
		t.Errorf("Render() should contain context 'Register', got: %q", stripped)
	}
	if !strings.Contains(stripped, "n new") {
		t.Errorf("Render() should contain key hints, got: %q", stripped)
	}
	if !strings.Contains(stripped, "1 due") {
		t.Errorf("Render() should contain notification, got: %q", stripped)
	}
	// Sections should be separated by │
	if !strings.Contains(stripped, "│") {
		t.Errorf("Render() should contain section separator │, got: %q", stripped)
	}
}

func TestStatusBar_Render_TruncatesLongHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Dashboard")
	// Create very long key hints that won't fit
	longHints := strings.Repeat("x", 200)
	sb.SetKeyHints(longHints)
	styles := NewStyles()
	styles.Resize(40, 24)

	result := sb.Render(styles, 40)
	resultWidth := lipgloss.Width(result)
	if resultWidth > 40 {
		t.Errorf("Render() width = %d, should not exceed 40", resultWidth)
	}
}

func TestStatusBar_Render_SmallWidth(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Dashboard")
	sb.SetKeyHints("? help  ctrl+q quit")
	sb.AddNotification("alert", NotificationAlert)
	styles := NewStyles()
	styles.Resize(30, 24)

	// Should not panic with very small width
	result := sb.Render(styles, 30)
	if result == "" {
		t.Error("Render() should produce output even at small width")
	}
}

func TestStatusBar_Render_MultipleNotifications(t *testing.T) {
	sb := NewStatusBar()
	sb.AddNotification("2 scheduled due", NotificationAlert)
	sb.AddNotification("File saved", NotificationInfo)
	styles := NewStyles()
	styles.Resize(120, 24)

	result := sb.Render(styles, 120)
	stripped := StripAnsi(result)

	if !strings.Contains(stripped, "2 scheduled due") {
		t.Errorf("Render() should contain first notification, got: %q", stripped)
	}
	if !strings.Contains(stripped, "File saved") {
		t.Errorf("Render() should contain second notification, got: %q", stripped)
	}
}

func TestStatusBar_Render_ContextOnly(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Reports")
	styles := NewStyles()
	styles.Resize(80, 24)

	result := sb.Render(styles, 80)
	stripped := StripAnsi(result)

	if !strings.Contains(stripped, "Reports") {
		t.Errorf("Render() should contain context, got: %q", stripped)
	}
}

func TestStatusBar_Render_NotificationsOnly(t *testing.T) {
	sb := NewStatusBar()
	sb.AddNotification("3 due", NotificationAlert)
	styles := NewStyles()
	styles.Resize(80, 24)

	result := sb.Render(styles, 80)
	stripped := StripAnsi(result)

	if !strings.Contains(stripped, "3 due") {
		t.Errorf("Render() should contain notification, got: %q", stripped)
	}
}

// Text wider than the bar must not wrap: lipgloss wraps text wider than
// Width, and a second row pushes the frame past the terminal height.
func TestStatusBar_Render_OverflowStaysOneRow(t *testing.T) {
	long := strings.Repeat("price refresh failed for EXMPL ", 8)
	tests := []struct {
		name  string
		setup func(sb *StatusBar)
	}{
		{"notification", func(sb *StatusBar) { sb.AddNotification(long, NotificationAlert) }},
		{"toast", func(sb *StatusBar) { sb.SetToast(long, NotificationInfo) }},
		{"sticky", func(sb *StatusBar) { sb.SetSticky(long, NotificationAlert) }},
		{"context", func(sb *StatusBar) { sb.SetContext(long) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := NewStatusBar()
			sb.SetContext("Dashboard – example.tdb")
			sb.SetKeyHints("↑↓ navigate  ? help  ctrl+q quit")
			tt.setup(sb)
			styles := NewStyles()
			styles.Resize(80, 24)

			got := sb.Render(styles, 80)
			if strings.Contains(got, "\n") {
				t.Fatalf("Render() wrapped to %d rows: %q", strings.Count(got, "\n")+1, StripAnsi(got))
			}
			if w := lipgloss.Width(got); w != 80 {
				t.Errorf("Render() width = %d, want 80", w)
			}
			if !strings.Contains(StripAnsi(got), "...") {
				t.Errorf("Render() = %q, want a \"...\" where the text is cut", StripAnsi(got))
			}
		})
	}
}

// The hints go first. Then the cut comes from the right, so the context
// and the start of the notification stay.
func TestStatusBar_Render_OverflowCutsFromTheRight(t *testing.T) {
	sb := NewStatusBar()
	sb.SetContext("Register")
	sb.SetKeyHints("n new  d delete  ? help")
	sb.AddNotification("Cannot edit reconciled transaction "+strings.Repeat("x", 100), NotificationAlert)
	styles := NewStyles()
	styles.Resize(60, 24)

	rendered := sb.Render(styles, 60)
	got := StripAnsi(rendered)
	if strings.Contains(got, "\n") || lipgloss.Width(rendered) != 60 {
		t.Fatalf("Render() = %q, want one row 60 cells wide", got)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "Register │ Cannot edit") {
		t.Errorf("Render() = %q, want the context, then the start of the notification", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "...") {
		t.Errorf("Render() = %q, want the notification cut with \"...\" at the right", got)
	}
	if strings.Contains(got, "n new") {
		t.Errorf("Render() = %q, want the hints dropped", got)
	}
}

// A line break in the text (an errors.Join error in a toast, for example)
// must not start a second row.
func TestStatusBar_Render_LineBreaksStayOneRow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(sb *StatusBar)
	}{
		{"toast", func(sb *StatusBar) { sb.SetToast("line one\nline two", NotificationAlert) }},
		{"notification", func(sb *StatusBar) { sb.AddNotification("a\r\nb", NotificationInfo) }},
		{"sticky", func(sb *StatusBar) { sb.SetSticky("a\rb", NotificationAlert) }},
		{"context", func(sb *StatusBar) { sb.SetContext("Dash\nboard") }},
		{"hints", func(sb *StatusBar) { sb.SetKeyHints("? help\tctrl+q quit\n") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := NewStatusBar()
			sb.SetContext("Register")
			tt.setup(sb)
			styles := NewStyles()
			styles.Resize(80, 24)

			got := sb.Render(styles, 80)
			if strings.Contains(got, "\n") {
				t.Fatalf("Render() wrapped to %d rows: %q", strings.Count(got, "\n")+1, StripAnsi(got))
			}
			if w := lipgloss.Width(got); w != 80 {
				t.Errorf("Render() width = %d, want 80", w)
			}
		})
	}
}

// Every width gives one row of exactly that width and valid UTF-8, also
// where the hint cut falls inside a multi-byte arrow.
func TestStatusBar_Render_OneRowAtAnyWidth(t *testing.T) {
	hints := "↑↓ navigate  ←→ collapse/expand  enter select"
	tests := []struct {
		name  string
		setup func(sb *StatusBar)
	}{
		{"hints only", func(sb *StatusBar) {}},
		{"all sections", func(sb *StatusBar) {
			sb.SetContext("Dashboard")
			sb.AddNotification("2 scheduled due", NotificationAlert)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := NewStatusBar()
			sb.SetKeyHints(hints)
			tt.setup(sb)
			styles := NewStyles()

			for width := 1; width <= 100; width++ {
				got := sb.Render(styles, width)
				if strings.Contains(got, "\n") {
					t.Errorf("width %d: Render() wrapped: %q", width, StripAnsi(got))
				}
				if w := lipgloss.Width(got); w != width {
					t.Errorf("width %d: Render() width = %d", width, w)
				}
				if !utf8.ValidString(got) {
					t.Errorf("width %d: Render() is not valid UTF-8: %q", width, got)
				}
			}
		})
	}
}

func TestNotificationLevel_Values(t *testing.T) {
	if NotificationInfo != 0 {
		t.Errorf("NotificationInfo = %d, want 0", NotificationInfo)
	}
	if NotificationAlert != 1 {
		t.Errorf("NotificationAlert = %d, want 1", NotificationAlert)
	}
}

// TestStatusBar_NewBar_NoToast asserts a fresh status bar has no toast.
func TestStatusBar_NewBar_NoToast(t *testing.T) {
	sb := NewStatusBar()
	if sb.Toast() != nil {
		t.Errorf("Toast() = %+v, want nil", sb.Toast())
	}
}

// TestStatusBar_SetToast verifies SetToast / Toast / ClearToast.
func TestStatusBar_SetToast(t *testing.T) {
	sb := NewStatusBar()

	sb.SetToast("theme load failed", NotificationAlert)
	got := sb.Toast()
	if got == nil {
		t.Fatal("Toast() = nil, want non-nil after SetToast")
	}
	if got.Text != "theme load failed" {
		t.Errorf("Toast().Text = %q, want %q", got.Text, "theme load failed")
	}
	if got.Level != NotificationAlert {
		t.Errorf("Toast().Level = %d, want %d", got.Level, NotificationAlert)
	}

	sb.ClearToast()
	if sb.Toast() != nil {
		t.Errorf("Toast() = %+v after ClearToast, want nil", sb.Toast())
	}
}

// TestStatusBar_SetToast_Replaces verifies a second SetToast replaces
// the first rather than queuing.
func TestStatusBar_SetToast_Replaces(t *testing.T) {
	sb := NewStatusBar()

	sb.SetToast("first", NotificationInfo)
	sb.SetToast("second", NotificationAlert)

	got := sb.Toast()
	if got == nil || got.Text != "second" {
		t.Errorf("Toast().Text = %v, want %q (replacement)", got, "second")
	}
	if got != nil && got.Level != NotificationAlert {
		t.Errorf("Toast().Level = %d, want %d", got.Level, NotificationAlert)
	}
}

// TestStatusBar_Render_WithToast asserts the toast appears in rendered
// output.
func TestStatusBar_Render_WithToast(t *testing.T) {
	sb := NewStatusBar()
	sb.SetToast("Theme 'broken': 2 issues, see log", NotificationAlert)
	styles := NewStyles()
	styles.Resize(120, 24)

	result := sb.Render(styles, 120)
	stripped := StripAnsi(result)
	if !strings.Contains(stripped, "Theme 'broken': 2 issues, see log") {
		t.Errorf("Render() should contain toast text, got: %q", stripped)
	}
}

// TestStatusBar_Render_ToastPersistsAcrossRenders confirms the toast
// stays put across consecutive Render calls (no auto-clearing on
// render).
func TestStatusBar_Render_ToastPersistsAcrossRenders(t *testing.T) {
	sb := NewStatusBar()
	sb.SetToast("hello", NotificationInfo)
	styles := NewStyles()
	styles.Resize(80, 24)

	for i := range 3 {
		result := sb.Render(styles, 80)
		stripped := StripAnsi(result)
		if !strings.Contains(stripped, "hello") {
			t.Errorf("render #%d missing toast: %q", i, stripped)
		}
	}
}

// TestStatusBar_Render_ToastOverridesNotifications asserts that while a
// toast is set the notifications slot shows only the toast text. Once
// cleared, the queued notifications resurface.
func TestStatusBar_Render_ToastOverridesNotifications(t *testing.T) {
	sb := NewStatusBar()
	sb.AddNotification("3 scheduled due", NotificationAlert)
	sb.SetToast("file saved", NotificationInfo)
	styles := NewStyles()
	styles.Resize(120, 24)

	result := sb.Render(styles, 120)
	stripped := StripAnsi(result)

	if !strings.Contains(stripped, "file saved") {
		t.Errorf("Render() should contain toast text, got: %q", stripped)
	}
	if strings.Contains(stripped, "3 scheduled due") {
		t.Errorf("Render() should NOT contain notification while toast is set, got: %q", stripped)
	}

	sb.ClearToast()
	result = sb.Render(styles, 120)
	stripped = StripAnsi(result)
	if !strings.Contains(stripped, "3 scheduled due") {
		t.Errorf("Render() should contain notification after ClearToast, got: %q", stripped)
	}
	if strings.Contains(stripped, "file saved") {
		t.Errorf("Render() should NOT contain toast text after ClearToast, got: %q", stripped)
	}
}

// TestStatusBar_AddNotificationWithID_UniqueIDs ensures each call returns
// a distinct, non-zero ID.
func TestStatusBar_AddNotificationWithID_UniqueIDs(t *testing.T) {
	sb := NewStatusBar()

	id1 := sb.AddNotificationWithID("Updating prices…", NotificationInfo)
	id2 := sb.AddNotificationWithID("Importing…", NotificationInfo)

	if id1 == 0 {
		t.Errorf("first ID = 0, want non-zero")
	}
	if id1 == id2 {
		t.Errorf("IDs not unique: id1 = %d, id2 = %d", id1, id2)
	}
}

// TestStatusBar_RemoveNotification_RemovesMatchingID asserts that
// RemoveNotification removes only the entry with the given ID.
func TestStatusBar_RemoveNotification_RemovesMatchingID(t *testing.T) {
	sb := NewStatusBar()

	id1 := sb.AddNotificationWithID("Updating prices…", NotificationInfo)
	sb.AddNotification("Other", NotificationInfo)

	sb.RemoveNotification(id1)

	notes := sb.Notifications()
	if len(notes) != 1 {
		t.Fatalf("Notifications() length = %d, want 1", len(notes))
	}
	if notes[0].Text != "Other" {
		t.Errorf("remaining notification text = %q, want %q", notes[0].Text, "Other")
	}
}

// TestStatusBar_RemoveNotification_UnknownIDIsNoOp asserts that calling
// RemoveNotification with an ID that has already been removed (or was
// never added) leaves the queue unchanged.
func TestStatusBar_RemoveNotification_UnknownIDIsNoOp(t *testing.T) {
	sb := NewStatusBar()

	id := sb.AddNotificationWithID("Updating prices…", NotificationInfo)
	sb.RemoveNotification(id)
	// Removing the same ID again should be a no-op.
	sb.RemoveNotification(id)
	// As should removing a never-issued ID.
	sb.RemoveNotification(99999)

	if got := len(sb.Notifications()); got != 0 {
		t.Errorf("Notifications() length = %d, want 0", got)
	}
}

// TestStatusBar_RemoveNotification_SurvivesEviction asserts that an ID
// remains usable for RemoveNotification after that entry has been
// evicted by maxNotifications: it simply becomes a no-op rather than
// removing some unrelated entry.
func TestStatusBar_RemoveNotification_SurvivesEviction(t *testing.T) {
	sb := NewStatusBar()

	evictedID := sb.AddNotificationWithID("first (will be evicted)", NotificationInfo)
	for i := range maxNotifications {
		sb.AddNotification("filler", NotificationInfo)
		_ = i
	}

	// The first notification should now be gone (evicted).
	for _, n := range sb.Notifications() {
		if n.Text == "first (will be evicted)" {
			t.Fatalf("expected first notification to be evicted, but it is still present")
		}
	}

	before := len(sb.Notifications())
	sb.RemoveNotification(evictedID)
	if got := len(sb.Notifications()); got != before {
		t.Errorf("RemoveNotification(evictedID) changed length from %d to %d, want no-op", before, got)
	}
}

// TestClearToastCmd_FiresClearMsg drives the tea.Cmd produced by
// ClearToastCmd with a tiny ToastDuration so the test doesn't wait the
// real ~5s; the resulting message must be a ToastClearMsg.
func TestClearToastCmd_FiresClearMsg(t *testing.T) {
	orig := ToastDuration
	ToastDuration = 1 * time.Millisecond
	t.Cleanup(func() { ToastDuration = orig })

	cmd := ClearToastCmd()
	if cmd == nil {
		t.Fatal("ClearToastCmd() returned nil")
	}

	msg := cmd()
	if _, ok := msg.(ToastClearMsg); !ok {
		t.Errorf("cmd() = %T, want ToastClearMsg", msg)
	}
}

// A sticky notification outlasts ClearNotifications, shows ahead of the
// queued ones, and goes only with ClearSticky.
func TestStatusBar_Sticky(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSticky("Startup repair failed", NotificationAlert)
	sb.AddNotification("1 scheduled due", NotificationAlert)

	render := func() string { return StripAnsi(sb.Render(NewStyles(), 200)) }
	got := render()
	if i, j := strings.Index(got, "Startup repair failed"), strings.Index(got, "1 scheduled due"); i < 0 || j < 0 || i > j {
		t.Errorf("render = %q, want the sticky alert before the queued one", got)
	}

	sb.ClearNotifications()
	if got := render(); !strings.Contains(got, "Startup repair failed") || strings.Contains(got, "scheduled due") {
		t.Errorf("after ClearNotifications render = %q, want only the sticky alert", got)
	}

	sb.ClearSticky()
	if got := render(); strings.Contains(got, "Startup repair failed") {
		t.Errorf("after ClearSticky render = %q", got)
	}
}
