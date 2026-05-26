package widget

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// maxNotifications is the maximum number of notifications displayed in the status bar.
const maxNotifications = 3

// ToastDuration is how long a toast stays visible before auto-clearing.
// Phase 9 (TH-031): the user-facing default is ~5s; tests override the
// var below to avoid waiting in real time.
var ToastDuration = 5 * time.Second

// Notification represents an alert or notification shown in the status bar.
type Notification struct {
	Text  string
	Level NotificationLevel
	// id is an opaque handle returned by AddNotificationWithID and
	// consumed by RemoveNotification. Zero means "no ID assigned"
	// (notifications added via AddNotification leave it zero).
	id int
}

// NotificationLevel indicates the severity of a notification.
type NotificationLevel int

const (
	// NotificationInfo is for informational notifications.
	NotificationInfo NotificationLevel = iota
	// NotificationAlert is for alerts requiring attention (e.g., scheduled transactions due).
	NotificationAlert
)

// Toast is a transient message shown in place of regular notifications
// for a short window (see ToastDuration). Used by Phase 9 to surface
// theme-load issues and similar one-shot events.
type Toast struct {
	Text  string
	Level NotificationLevel
}

// ToastClearMsg signals the status bar should clear its current toast.
// Emitted by the tea.Cmd returned from ClearToastCmd after ToastDuration.
type ToastClearMsg struct{}

// ClearToastCmd returns a tea.Cmd that fires a ToastClearMsg after
// ToastDuration. Pair with StatusBar.SetToast: set the toast, return
// this cmd, and let the message handler call StatusBar.ClearToast.
func ClearToastCmd() tea.Cmd {
	return tea.Tick(ToastDuration, func(_ time.Time) tea.Msg {
		return ToastClearMsg{}
	})
}

// StatusBar manages the bottom status bar state and rendering.
type StatusBar struct {
	// Context is the current view/context label (e.g., "Dashboard", "Checking").
	context string

	// KeyHints is the key hints string for the current context.
	keyHints string

	// Notifications are alert messages shown on the right side.
	notifications []Notification

	// toast is an optional transient message that takes the
	// notifications slot until cleared. Notifications underneath
	// remain queued and reappear once the toast is cleared.
	toast *Toast

	// nextID is a monotonic counter for AddNotificationWithID handles.
	nextID int
}

// NewStatusBar creates a new StatusBar with default state.
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetContext sets the context label displayed on the left side.
func (sb *StatusBar) SetContext(ctx string) {
	sb.context = ctx
}

// Context returns the current context label.
func (sb *StatusBar) Context() string {
	return sb.context
}

// SetKeyHints sets the key hints string.
func (sb *StatusBar) SetKeyHints(hints string) {
	sb.keyHints = hints
}

// KeyHints returns the current key hints string.
func (sb *StatusBar) KeyHints() string {
	return sb.keyHints
}

// AddNotification adds a notification to the status bar.
// If the maximum number of notifications is reached, the oldest is removed.
func (sb *StatusBar) AddNotification(text string, level NotificationLevel) {
	sb.notifications = append(sb.notifications, Notification{
		Text:  text,
		Level: level,
	})
	if len(sb.notifications) > maxNotifications {
		sb.notifications = sb.notifications[len(sb.notifications)-maxNotifications:]
	}
}

// AddNotificationWithID adds a notification and returns an opaque ID
// that can later be passed to RemoveNotification. Behaves like
// AddNotification with respect to maxNotifications: an evicted entry's
// ID becomes inert (RemoveNotification on it is a no-op).
func (sb *StatusBar) AddNotificationWithID(text string, level NotificationLevel) int {
	sb.nextID++
	id := sb.nextID
	sb.notifications = append(sb.notifications, Notification{
		Text:  text,
		Level: level,
		id:    id,
	})
	if len(sb.notifications) > maxNotifications {
		sb.notifications = sb.notifications[len(sb.notifications)-maxNotifications:]
	}
	return id
}

// RemoveNotification removes the notification with the given ID. If no
// notification with that ID is currently in the queue (e.g., it was
// evicted, never existed, or was already removed) the call is a no-op.
func (sb *StatusBar) RemoveNotification(id int) {
	if id == 0 {
		return
	}
	for i, n := range sb.notifications {
		if n.id == id {
			sb.notifications = append(sb.notifications[:i], sb.notifications[i+1:]...)
			return
		}
	}
}

// ClearNotifications removes all notifications.
func (sb *StatusBar) ClearNotifications() {
	sb.notifications = nil
}

// Notifications returns the current notifications.
func (sb *StatusBar) Notifications() []Notification {
	return sb.notifications
}

// SetToast replaces the current toast with the given text and level.
// While set, the toast is rendered in place of any pending notifications.
func (sb *StatusBar) SetToast(text string, level NotificationLevel) {
	sb.toast = &Toast{Text: text, Level: level}
}

// ClearToast removes the active toast so queued notifications can be
// rendered again.
func (sb *StatusBar) ClearToast() {
	sb.toast = nil
}

// Toast returns the active toast or nil.
func (sb *StatusBar) Toast() *Toast {
	return sb.toast
}

// Render renders the status bar at the given width using the provided styles.
//
// Each inner segment (context, key hints, notifications, separators, and
// the trailing width-fill spaces) is rendered with the StatusBar's
// background already established. This is required because lipgloss
// emits a reset (`\e[m`) at the end of every Render; without inheriting
// the bg on the inner segments, the first reset would clobber the
// outer StatusBar background and everything to the right of it would
// fall back to the terminal default — which on the Turbo Vision theme
// shows the blue desktop bleeding through the gray bar.
func (sb *StatusBar) Render(styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	// barBase is the StatusBar style stripped of padding and width —
	// the outer Render below applies the (0, 1) padding and the
	// terminal width fill once, so inner segments must not duplicate
	// either (an inherited Width would pad each segment to the full
	// bar and force newlines when joined).
	barBase := styles.StatusBar.Padding(0).UnsetWidth()

	left := sb.renderContext(styles, barBase)
	right := sb.renderNotifications(styles, barBase)
	center := sb.keyHints

	// Calculate available space for key hints
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	centerWidth := lipgloss.Width(center)

	// Add separators if sections are non-empty
	separatorWidth := 0
	if leftWidth > 0 && centerWidth > 0 {
		separatorWidth += 3 // " │ "
	}
	if rightWidth > 0 && (centerWidth > 0 || leftWidth > 0) {
		separatorWidth += 3 // " │ "
	}

	// Padding from the StatusBar style (0, 1) = 2 chars horizontal padding
	padding := 2
	usedWidth := leftWidth + rightWidth + separatorWidth + padding

	// Truncate center hints if they don't fit
	availableCenter := max(width-usedWidth, 0)
	if centerWidth > availableCenter {
		if availableCenter > 3 {
			center = center[:availableCenter-3] + "..."
		} else {
			center = ""
		}
		centerWidth = lipgloss.Width(center)
	}

	// Render the center key hints with the StatusBar bg inherited.
	var renderedCenter string
	if centerWidth > 0 {
		renderedCenter = barBase.Render(center)
	}

	separator := barBase.Render(" │ ")

	// Build the bar content from segments that all carry the StatusBar
	// background, joined with separators that do too.
	var parts []string
	if leftWidth > 0 {
		parts = append(parts, left)
	}
	if centerWidth > 0 {
		parts = append(parts, renderedCenter)
	}
	if rightWidth > 0 {
		parts = append(parts, right)
	}

	content := strings.Join(parts, separator)

	// Pad to fill width with bg-painted spaces (accounting for the outer
	// style's horizontal padding). Rendering the fill through barBase
	// keeps the gray bg continuous to the right edge.
	contentWidth := lipgloss.Width(content)
	innerWidth := width - padding
	if contentWidth < innerWidth {
		content += barBase.Render(strings.Repeat(" ", innerWidth-contentWidth))
	}

	return styles.StatusBar.Width(width).Render(content)
}

// renderContext renders the context section (left side) with the
// StatusBar background inherited so the segment's trailing reset
// doesn't strip the bar's bg from everything that follows.
func (sb *StatusBar) renderContext(_ Styles, barBase lipgloss.Style) string {
	if sb.context == "" {
		return ""
	}
	return barBase.Bold(true).Render(sb.context)
}

// renderNotifications renders the notification section (right side).
// An active toast takes precedence over the queued notifications: while
// the toast is set, only the toast text appears in this slot. Each
// segment is rendered with the StatusBar bg inherited (see Render).
func (sb *StatusBar) renderNotifications(_ Styles, barBase lipgloss.Style) string {
	alertStyle := barBase.Foreground(ColorAlert).Bold(true)

	if sb.toast != nil {
		if sb.toast.Level == NotificationAlert {
			return alertStyle.Render(sb.toast.Text)
		}
		return barBase.Render(sb.toast.Text)
	}

	if len(sb.notifications) == 0 {
		return ""
	}

	var parts []string
	for _, n := range sb.notifications {
		switch n.Level {
		case NotificationAlert:
			parts = append(parts, alertStyle.Render(n.Text))
		default:
			parts = append(parts, barBase.Render(n.Text))
		}
	}

	return strings.Join(parts, barBase.Render("  "))
}
