package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// maxNotifications is the maximum number of notifications displayed in the status bar.
const maxNotifications = 3

// Notification represents an alert or notification shown in the status bar.
type Notification struct {
	Text  string
	Level NotificationLevel
}

// NotificationLevel indicates the severity of a notification.
type NotificationLevel int

const (
	// NotificationInfo is for informational notifications.
	NotificationInfo NotificationLevel = iota
	// NotificationAlert is for alerts requiring attention (e.g., scheduled transactions due).
	NotificationAlert
)

// StatusBar manages the bottom status bar state and rendering.
type StatusBar struct {
	// Context is the current view/context label (e.g., "Dashboard", "Checking").
	context string

	// KeyHints is the key hints string for the current context.
	keyHints string

	// Notifications are alert messages shown on the right side.
	notifications []Notification
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

// ClearNotifications removes all notifications.
func (sb *StatusBar) ClearNotifications() {
	sb.notifications = nil
}

// Notifications returns the current notifications.
func (sb *StatusBar) Notifications() []Notification {
	return sb.notifications
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
func (sb *StatusBar) renderNotifications(styles Styles) string {
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
