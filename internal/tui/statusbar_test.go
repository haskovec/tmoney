package tui

import (
	"strings"
	"testing"
	"time"

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
	stripped := stripAnsi(result)
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
	stripped := stripAnsi(result)
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
	stripped := stripAnsi(result)
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
	stripped := stripAnsi(result)

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
	stripped := stripAnsi(result)

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
	stripped := stripAnsi(result)

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
	stripped := stripAnsi(result)

	if !strings.Contains(stripped, "3 due") {
		t.Errorf("Render() should contain notification, got: %q", stripped)
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
	stripped := stripAnsi(result)
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
		stripped := stripAnsi(result)
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
	stripped := stripAnsi(result)

	if !strings.Contains(stripped, "file saved") {
		t.Errorf("Render() should contain toast text, got: %q", stripped)
	}
	if strings.Contains(stripped, "3 scheduled due") {
		t.Errorf("Render() should NOT contain notification while toast is set, got: %q", stripped)
	}

	sb.ClearToast()
	result = sb.Render(styles, 120)
	stripped = stripAnsi(result)
	if !strings.Contains(stripped, "3 scheduled due") {
		t.Errorf("Render() should contain notification after ClearToast, got: %q", stripped)
	}
	if strings.Contains(stripped, "file saved") {
		t.Errorf("Render() should NOT contain toast text after ClearToast, got: %q", stripped)
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
