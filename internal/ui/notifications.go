// Package ui provides desktop notification support.
package ui

import (
	"os/exec"
	"runtime"
)

// NotificationManager handles desktop notifications.
type NotificationManager struct {
	enabled bool
}

// NewNotificationManager creates a new notification manager.
func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		enabled: true,
	}
}

// SetEnabled enables or disables notifications.
func (nm *NotificationManager) SetEnabled(enabled bool) {
	nm.enabled = enabled
}

// Notify sends a desktop notification.
func (nm *NotificationManager) Notify(title, message string) {
	if !nm.enabled {
		return
	}

	go nm.sendNotification(title, message)
}

// NotifyTransferComplete sends a notification for completed transfer.
func (nm *NotificationManager) NotifyTransferComplete(filename string, uploaded bool) {
	action := "téléchargé"
	if uploaded {
		action = "envoyé"
	}
	nm.Notify("Transfert terminé", filename+" "+action+" avec succès")
}

// NotifyTransferFailed sends a notification for failed transfer.
func (nm *NotificationManager) NotifyTransferFailed(filename string, err error) {
	nm.Notify("Échec du transfert", filename+" : "+err.Error())
}

// NotifySyncComplete sends a notification for completed sync.
func (nm *NotificationManager) NotifySyncComplete(uploaded, downloaded, deleted int) {
	nm.Notify("Synchronisation terminée",
		"Envoyés : "+itoa(uploaded)+
			", Téléchargés : "+itoa(downloaded)+
			", Supprimés : "+itoa(deleted))
}

// sendNotification sends the actual notification based on OS.
func (nm *NotificationManager) sendNotification(title, message string) {
	switch runtime.GOOS {
	case "linux":
		nm.sendLinuxNotification(title, message)
	case "darwin":
		nm.sendMacNotification(title, message)
	case "windows":
		nm.sendWindowsNotification(title, message)
	}
}

// sendLinuxNotification sends a notification on Linux using notify-send.
func (nm *NotificationManager) sendLinuxNotification(title, message string) {
	exec.Command("notify-send", "-a", "Secure FTP", title, message).Run()
}

// sendMacNotification sends a notification on macOS.
func (nm *NotificationManager) sendMacNotification(title, message string) {
	// Escape double quotes to prevent command injection
	title = escapeAppleScript(title)
	message = escapeAppleScript(message)
	script := `display notification "` + message + `" with title "` + title + `"`
	exec.Command("osascript", "-e", script).Run()
}

// escapeAppleScript escapes special characters for AppleScript strings.
func escapeAppleScript(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '"':
			result += `\"`
		case '\\':
			result += `\\`
		default:
			result += string(c)
		}
	}
	return result
}

// sendWindowsNotification sends a notification on Windows.
func (nm *NotificationManager) sendWindowsNotification(title, message string) {
	// Windows notifications require more complex setup
	// This is a placeholder - would use toast notifications in production
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}
