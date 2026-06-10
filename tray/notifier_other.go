//go:build !windows

package tray

// ToastNotifier is a no-op notifier on non-Windows platforms.
type ToastNotifier struct{}

// NewToastNotifier creates a no-op notifier.
func NewToastNotifier(icon string) *ToastNotifier {
	return &ToastNotifier{}
}

// Notify ignores notifications on non-Windows platforms.
func (n *ToastNotifier) Notify(notification Notification) error {
	return nil
}
