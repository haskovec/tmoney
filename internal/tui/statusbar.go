package tui

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
func (sb *StatusBar) Render(styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	left := sb.renderContext(styles)
	right := sb.renderNotifications(styles)
	center := sb.keyHints

	// Calculate available space for key hints
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)

	// Add separators if sections are non-empty
	separatorWidth := 0
	if leftWidth > 0 && len(center) > 0 {
		separatorWidth += 3 // " │ "
	}
	if rightWidth > 0 && (len(center) > 0 || leftWidth > 0) {
		separatorWidth += 3 // " │ "
	}

	// Padding from the StatusBar style (0, 1) = 2 chars horizontal padding
	padding := 2
	usedWidth := leftWidth + rightWidth + separatorWidth + padding

	// Truncate center hints if they don't fit
	availableCenter := max(width-usedWidth, 0)
	if len(center) > availableCenter {
		if availableCenter > 3 {
			center = center[:availableCenter-3] + "..."
		} else {
			center = ""
		}
	}

	// Build the bar content
	var parts []string
	if leftWidth > 0 {
		parts = append(parts, left)
	}
	if len(center) > 0 {
		parts = append(parts, center)
	}
	if rightWidth > 0 {
		parts = append(parts, right)
	}

	content := strings.Join(parts, " │ ")

	// Pad to fill width (accounting for style padding)
	contentWidth := lipgloss.Width(content)
	innerWidth := width - padding
	if contentWidth < innerWidth {
		content = content + strings.Repeat(" ", innerWidth-contentWidth)
	}

	return styles.StatusBar.Width(width).Render(content)
}

// renderContext renders the context section (left side).
func (sb *StatusBar) renderContext(styles Styles) string {
	if sb.context == "" {
		return ""
	}
	return styles.Bold.Render(sb.context)
}

// renderNotifications renders the notification section (right side).
// An active toast takes precedence over the queued notifications: while
// the toast is set, only the toast text appears in this slot.
func (sb *StatusBar) renderNotifications(styles Styles) string {
	if sb.toast != nil {
		text := sb.toast.Text
		if sb.toast.Level == NotificationAlert {
			text = styles.Alert.Render(text)
		}
		return text
	}

	if len(sb.notifications) == 0 {
		return ""
	}

	var parts []string
	for _, n := range sb.notifications {
		text := n.Text
		switch n.Level {
		case NotificationAlert:
			text = styles.Alert.Render(text)
		default:
			// Info level: no special styling
		}
		parts = append(parts, text)
	}

	return strings.Join(parts, "  ")
}
