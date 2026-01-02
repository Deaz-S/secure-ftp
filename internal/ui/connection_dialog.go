// Package ui provides the connection dialog.
package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/config"
)

// ConnectionDialog handles server connection setup.
type ConnectionDialog struct {
	window     fyne.Window
	configMgr  *config.ConfigManager
	onConnect  func(*config.ConnectionProfile, string)

	// Form fields
	profileSelect *widget.Select
	protocolSelect *widget.Select
	hostEntry     *widget.Entry
	portEntry     *widget.Entry
	usernameEntry *widget.Entry
	passwordEntry *widget.Entry
	remoteDirEntry *widget.Entry
	tlsImplicitCheck *widget.Check
	saveProfileCheck *widget.Check
	profileNameEntry *widget.Entry
}

// NewConnectionDialog creates a new connection dialog.
func NewConnectionDialog(parent fyne.Window, configMgr *config.ConfigManager, onConnect func(*config.ConnectionProfile, string)) *ConnectionDialog {
	return &ConnectionDialog{
		window:    parent,
		configMgr: configMgr,
		onConnect: onConnect,
	}
}

// Show displays the connection dialog.
func (cd *ConnectionDialog) Show() {
	cd.buildForm()
}

// buildForm constructs the dialog form.
func (cd *ConnectionDialog) buildForm() {
	// Create ALL widgets first (without callbacks to avoid nil pointer issues)

	// Connection fields
	cd.hostEntry = widget.NewEntry()
	cd.hostEntry.SetPlaceHolder("hostname or IP address")

	cd.portEntry = widget.NewEntry()
	cd.portEntry.SetPlaceHolder("22")
	cd.portEntry.SetText("22")

	cd.usernameEntry = widget.NewEntry()
	cd.usernameEntry.SetPlaceHolder("username")

	cd.passwordEntry = widget.NewPasswordEntry()
	cd.passwordEntry.SetPlaceHolder("password")

	cd.remoteDirEntry = widget.NewEntry()
	cd.remoteDirEntry.SetPlaceHolder("/home/user (optional)")

	cd.tlsImplicitCheck = widget.NewCheck("Implicit TLS (port 990)", nil)
	cd.tlsImplicitCheck.Hide()

	// Save profile option
	cd.saveProfileCheck = widget.NewCheck("Save as profile", nil)
	cd.profileNameEntry = widget.NewEntry()
	cd.profileNameEntry.SetPlaceHolder("Profile name")
	cd.profileNameEntry.Hide()

	cd.saveProfileCheck.OnChanged = func(checked bool) {
		if checked {
			cd.profileNameEntry.Show()
		} else {
			cd.profileNameEntry.Hide()
		}
	}

	// Protocol selection (create before profile to avoid nil in clearForm)
	cd.protocolSelect = widget.NewSelect([]string{"SFTP", "FTPS", "FTP"}, cd.onProtocolSelected)

	// Profile selection
	profiles := cd.configMgr.GetProfiles()
	profileNames := make([]string, len(profiles)+1)
	profileNames[0] = "-- New Connection --"
	for i, p := range profiles {
		profileNames[i+1] = p.Name
	}
	cd.profileSelect = widget.NewSelect(profileNames, cd.onProfileSelected)

	// Now set initial selections (callbacks are safe now)
	cd.protocolSelect.SetSelectedIndex(0)
	cd.profileSelect.SetSelectedIndex(0)

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Profile:"),
		cd.profileSelect,
		widget.NewSeparator(),
		widget.NewLabel("Protocol:"),
		cd.protocolSelect,
		widget.NewLabel("Host:"),
		cd.hostEntry,
		widget.NewLabel("Port:"),
		cd.portEntry,
		cd.tlsImplicitCheck,
		widget.NewSeparator(),
		widget.NewLabel("Username:"),
		cd.usernameEntry,
		widget.NewLabel("Password:"),
		cd.passwordEntry,
		widget.NewSeparator(),
		widget.NewLabel("Remote Directory:"),
		cd.remoteDirEntry,
		widget.NewSeparator(),
		cd.saveProfileCheck,
		cd.profileNameEntry,
	)

	// Create custom dialog
	dlg := dialog.NewCustomConfirm("Connect to Server", "Connect", "Cancel", form,
		func(confirmed bool) {
			if confirmed {
				cd.handleConnect()
			}
		}, cd.window)

	dlg.Resize(fyne.NewSize(400, 550))
	dlg.Show()
}

// onProfileSelected handles profile selection.
func (cd *ConnectionDialog) onProfileSelected(selected string) {
	if selected == "-- New Connection --" {
		cd.clearForm()
		return
	}

	// Find and load profile
	profiles := cd.configMgr.GetProfiles()
	for _, p := range profiles {
		if p.Name == selected {
			cd.loadProfile(&p)
			return
		}
	}
}

// loadProfile fills the form with profile data.
func (cd *ConnectionDialog) loadProfile(profile *config.ConnectionProfile) {
	if profile.Protocol == "sftp" {
		cd.protocolSelect.SetSelectedIndex(0)
	} else {
		cd.protocolSelect.SetSelectedIndex(1)
	}

	cd.hostEntry.SetText(profile.Host)
	cd.portEntry.SetText(strconv.Itoa(profile.Port))
	cd.usernameEntry.SetText(profile.Username)
	cd.remoteDirEntry.SetText(profile.RemoteDir)
	cd.tlsImplicitCheck.SetChecked(profile.TLSImplicit)

	// Password is not stored, leave empty
	cd.passwordEntry.SetText("")
}

// clearForm resets the form to defaults.
func (cd *ConnectionDialog) clearForm() {
	cd.protocolSelect.SetSelectedIndex(0)
	cd.hostEntry.SetText("")
	cd.portEntry.SetText("22")
	cd.usernameEntry.SetText("")
	cd.passwordEntry.SetText("")
	cd.remoteDirEntry.SetText("")
	cd.tlsImplicitCheck.SetChecked(false)
	cd.saveProfileCheck.SetChecked(false)
	cd.profileNameEntry.SetText("")
	cd.profileNameEntry.Hide()
}

// onProtocolSelected handles protocol selection.
func (cd *ConnectionDialog) onProtocolSelected(selected string) {
	switch selected {
	case "SFTP":
		cd.portEntry.SetText("22")
		cd.tlsImplicitCheck.Hide()
	case "FTPS":
		if cd.tlsImplicitCheck.Checked {
			cd.portEntry.SetText("990")
		} else {
			cd.portEntry.SetText("21")
		}
		cd.tlsImplicitCheck.Show()
	case "FTP":
		cd.portEntry.SetText("21")
		cd.tlsImplicitCheck.Hide()
	}
}

// handleConnect processes the connection request.
func (cd *ConnectionDialog) handleConnect() {
	// Validate required fields
	if cd.hostEntry.Text == "" {
		dialog.ShowError(errMissingHost, cd.window)
		return
	}

	if cd.usernameEntry.Text == "" {
		dialog.ShowError(errMissingUsername, cd.window)
		return
	}

	if cd.passwordEntry.Text == "" {
		dialog.ShowError(errMissingPassword, cd.window)
		return
	}

	// Parse port
	port, err := strconv.Atoi(cd.portEntry.Text)
	if err != nil {
		port = 22
	}

	// Determine protocol
	protocol := "sftp"
	switch cd.protocolSelect.Selected {
	case "FTPS":
		protocol = "ftps"
	case "FTP":
		protocol = "ftp"
	}

	// Create profile
	profile := &config.ConnectionProfile{
		Name:        cd.profileNameEntry.Text,
		Protocol:    protocol,
		Host:        cd.hostEntry.Text,
		Port:        port,
		Username:    cd.usernameEntry.Text,
		RemoteDir:   cd.remoteDirEntry.Text,
		TLSImplicit: cd.tlsImplicitCheck.Checked,
	}

	// Save profile if requested
	if cd.saveProfileCheck.Checked && cd.profileNameEntry.Text != "" {
		cd.configMgr.AddProfile(*profile)
	}

	// Trigger connection
	if cd.onConnect != nil {
		cd.onConnect(profile, cd.passwordEntry.Text)
	}
}

// Error messages
var (
	errMissingHost     = &connectionError{"Host is required"}
	errMissingUsername = &connectionError{"Username is required"}
	errMissingPassword = &connectionError{"Password is required"}
)

type connectionError struct {
	message string
}

func (e *connectionError) Error() string {
	return e.message
}
