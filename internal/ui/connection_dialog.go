// Package ui provides the connection dialog.
package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/config"
)

// ConnectionDialog handles server connection setup.
type ConnectionDialog struct {
	window        fyne.Window
	configMgr     *config.ConfigManager
	credentialsMgr *config.CredentialsManager
	onConnect     func(*config.ConnectionProfile, string)

	// Currently selected profile ID (for password loading)
	selectedProfileID string

	// Form fields
	profileSelect     *widget.Select
	deleteProfileBtn  *widget.Button
	protocolSelect    *widget.Select
	hostEntry         *widget.Entry
	portEntry         *widget.Entry
	usernameEntry     *widget.Entry
	passwordEntry     *widget.Entry
	privateKeyEntry   *widget.Entry
	privateKeyBtn     *widget.Button
	remoteDirEntry    *widget.Entry
	tlsImplicitCheck  *widget.Check
	saveProfileCheck  *widget.Check
	savePasswordCheck *widget.Check
	profileNameEntry  *widget.Entry
}

// NewConnectionDialog creates a new connection dialog.
func NewConnectionDialog(parent fyne.Window, configMgr *config.ConfigManager, credsMgr *config.CredentialsManager, onConnect func(*config.ConnectionProfile, string)) *ConnectionDialog {
	return &ConnectionDialog{
		window:         parent,
		configMgr:      configMgr,
		credentialsMgr: credsMgr,
		onConnect:      onConnect,
	}
}

// Show displays the connection dialog.
func (cd *ConnectionDialog) Show() {
	cd.buildForm()
}

// buildForm constructs the dialog form.
func (cd *ConnectionDialog) buildForm() {
	// Connection fields
	cd.hostEntry = widget.NewEntry()
	cd.hostEntry.SetPlaceHolder("nom d'hôte ou adresse IP")

	cd.portEntry = widget.NewEntry()
	cd.portEntry.SetPlaceHolder("22")
	cd.portEntry.SetText("22")

	cd.usernameEntry = widget.NewEntry()
	cd.usernameEntry.SetPlaceHolder("nom d'utilisateur")

	cd.passwordEntry = widget.NewPasswordEntry()
	cd.passwordEntry.SetPlaceHolder("mot de passe")

	// Private key path
	cd.privateKeyEntry = widget.NewEntry()
	cd.privateKeyEntry.SetPlaceHolder("~/.ssh/id_rsa (optionnel)")

	cd.privateKeyBtn = widget.NewButton("Parcourir...", func() {
		dlg := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			cd.privateKeyEntry.SetText(reader.URI().Path())
			reader.Close()
		}, cd.window)
		dlg.Show()
	})

	cd.remoteDirEntry = widget.NewEntry()
	cd.remoteDirEntry.SetPlaceHolder("/home/user (optionnel)")

	cd.tlsImplicitCheck = widget.NewCheck("TLS implicite (port 990)", nil)
	cd.tlsImplicitCheck.Hide()

	// Save profile option
	cd.saveProfileCheck = widget.NewCheck("Enregistrer comme profil", nil)
	cd.profileNameEntry = widget.NewEntry()
	cd.profileNameEntry.SetPlaceHolder("Nom du profil")
	cd.profileNameEntry.Hide()

	// Save password option
	cd.savePasswordCheck = widget.NewCheck("Mémoriser le mot de passe", nil)
	cd.savePasswordCheck.Hide()

	cd.saveProfileCheck.OnChanged = func(checked bool) {
		if checked {
			cd.profileNameEntry.Show()
			cd.savePasswordCheck.Show()
		} else {
			cd.profileNameEntry.Hide()
			cd.savePasswordCheck.Hide()
			cd.savePasswordCheck.SetChecked(false)
		}
	}

	// Protocol selection
	cd.protocolSelect = widget.NewSelect([]string{"SFTP", "FTPS", "FTP"}, cd.onProtocolSelected)

	// Profile selection
	profiles := cd.configMgr.GetProfiles()
	profileNames := make([]string, len(profiles)+1)
	profileNames[0] = "-- Nouvelle connexion --"
	for i, p := range profiles {
		profileNames[i+1] = p.Name
	}
	cd.profileSelect = widget.NewSelect(profileNames, cd.onProfileSelected)

	// Delete profile button (disabled by default)
	cd.deleteProfileBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), cd.deleteSelectedProfile)
	cd.deleteProfileBtn.Importance = widget.DangerImportance
	cd.deleteProfileBtn.Disable()

	// Initial selections
	cd.protocolSelect.SetSelectedIndex(0)
	cd.profileSelect.SetSelectedIndex(0)

	// Private key row
	privateKeyRow := container.NewBorder(nil, nil, nil, cd.privateKeyBtn, cd.privateKeyEntry)

	// Profile row with delete button
	profileRow := container.NewBorder(nil, nil, nil, cd.deleteProfileBtn, cd.profileSelect)

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Profil :"),
		profileRow,
		widget.NewSeparator(),
		widget.NewLabel("Protocole :"),
		cd.protocolSelect,
		widget.NewLabel("Hôte :"),
		cd.hostEntry,
		widget.NewLabel("Port :"),
		cd.portEntry,
		cd.tlsImplicitCheck,
		widget.NewSeparator(),
		widget.NewLabel("Nom d'utilisateur :"),
		cd.usernameEntry,
		widget.NewLabel("Mot de passe :"),
		cd.passwordEntry,
		widget.NewLabel("Clé privée (SSH) :"),
		privateKeyRow,
		widget.NewSeparator(),
		widget.NewLabel("Répertoire distant :"),
		cd.remoteDirEntry,
		widget.NewSeparator(),
		cd.saveProfileCheck,
		cd.profileNameEntry,
		cd.savePasswordCheck,
	)

	// Create scrollable container for small screens
	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(380, 500))

	// Create custom dialog
	dlg := dialog.NewCustomConfirm("Connexion au serveur", "Connexion", "Annuler", scroll,
		func(confirmed bool) {
			if confirmed {
				cd.handleConnect()
			}
		}, cd.window)

	dlg.Resize(fyne.NewSize(420, 600))
	dlg.Show()
}

func (cd *ConnectionDialog) onProfileSelected(selected string) {
	if selected == "-- Nouvelle connexion --" {
		cd.clearForm()
		cd.selectedProfileID = ""
		cd.deleteProfileBtn.Disable()
		return
	}

	profiles := cd.configMgr.GetProfiles()
	for _, p := range profiles {
		if p.Name == selected {
			cd.loadProfile(&p)
			cd.deleteProfileBtn.Enable()
			return
		}
	}
}

// loadProfile fills the form with profile data.
func (cd *ConnectionDialog) loadProfile(profile *config.ConnectionProfile) {
	cd.selectedProfileID = profile.ID

	switch profile.Protocol {
	case "sftp":
		cd.protocolSelect.SetSelectedIndex(0)
	case "ftps":
		cd.protocolSelect.SetSelectedIndex(1)
	case "ftp":
		cd.protocolSelect.SetSelectedIndex(2)
	}

	cd.hostEntry.SetText(profile.Host)
	cd.portEntry.SetText(strconv.Itoa(profile.Port))
	cd.usernameEntry.SetText(profile.Username)
	cd.remoteDirEntry.SetText(profile.RemoteDir)
	cd.tlsImplicitCheck.SetChecked(profile.TLSImplicit)
	cd.privateKeyEntry.SetText(profile.PrivateKeyPath)

	// Try to load saved password
	if cd.credentialsMgr != nil && profile.ID != "" {
		if password, err := cd.credentialsMgr.GetPassword(profile.ID); err == nil && password != "" {
			cd.passwordEntry.SetText(password)
		} else {
			cd.passwordEntry.SetText("")
		}
	} else {
		cd.passwordEntry.SetText("")
	}

	// Hide save options for existing profile
	cd.saveProfileCheck.SetChecked(false)
	cd.saveProfileCheck.Hide()
	cd.profileNameEntry.Hide()
	cd.savePasswordCheck.Hide()
}

// clearForm resets the form to defaults.
func (cd *ConnectionDialog) clearForm() {
	cd.protocolSelect.SetSelectedIndex(0)
	cd.hostEntry.SetText("")
	cd.portEntry.SetText("22")
	cd.usernameEntry.SetText("")
	cd.passwordEntry.SetText("")
	cd.privateKeyEntry.SetText("")
	cd.remoteDirEntry.SetText("")
	cd.tlsImplicitCheck.SetChecked(false)
	cd.saveProfileCheck.SetChecked(false)
	cd.saveProfileCheck.Show()
	cd.profileNameEntry.SetText("")
	cd.profileNameEntry.Hide()
	cd.savePasswordCheck.SetChecked(false)
	cd.savePasswordCheck.Hide()
}

// onProtocolSelected handles protocol selection.
func (cd *ConnectionDialog) onProtocolSelected(selected string) {
	switch selected {
	case "SFTP":
		cd.portEntry.SetText("22")
		cd.tlsImplicitCheck.Hide()
		cd.privateKeyEntry.Show()
		cd.privateKeyBtn.Show()
	case "FTPS":
		if cd.tlsImplicitCheck.Checked {
			cd.portEntry.SetText("990")
		} else {
			cd.portEntry.SetText("21")
		}
		cd.tlsImplicitCheck.Show()
		cd.privateKeyEntry.Hide()
		cd.privateKeyBtn.Hide()
	case "FTP":
		cd.portEntry.SetText("21")
		cd.tlsImplicitCheck.Hide()
		cd.privateKeyEntry.Hide()
		cd.privateKeyBtn.Hide()
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

	// Password can be empty if private key is provided
	if cd.passwordEntry.Text == "" && cd.privateKeyEntry.Text == "" {
		dialog.ShowError(errMissingAuth, cd.window)
		return
	}

	// Parse port with validation
	port, err := strconv.Atoi(cd.portEntry.Text)
	if err != nil || port < 1 || port > 65535 {
		dialog.ShowError(errInvalidPort, cd.window)
		return
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
		ID:             cd.selectedProfileID,
		Name:           cd.profileNameEntry.Text,
		Protocol:       protocol,
		Host:           cd.hostEntry.Text,
		Port:           port,
		Username:       cd.usernameEntry.Text,
		PrivateKeyPath: cd.privateKeyEntry.Text,
		RemoteDir:      cd.remoteDirEntry.Text,
		TLSImplicit:    cd.tlsImplicitCheck.Checked,
	}

	// Save profile if requested
	if cd.saveProfileCheck.Checked && cd.profileNameEntry.Text != "" {
		if err := cd.configMgr.AddProfile(*profile); err == nil {
			// Get the generated ID
			profiles := cd.configMgr.GetProfiles()
			for _, p := range profiles {
				if p.Name == profile.Name {
					profile.ID = p.ID
					break
				}
			}

			// Save password if requested
			if cd.savePasswordCheck.Checked && cd.credentialsMgr != nil && profile.ID != "" {
				cd.credentialsMgr.SetPassword(profile.ID, cd.passwordEntry.Text)
			}
		}
	}

	// Trigger connection
	if cd.onConnect != nil {
		cd.onConnect(profile, cd.passwordEntry.Text)
	}
}

// Error messages
var (
	errMissingHost     = &connectionError{"L'hôte est requis"}
	errMissingUsername = &connectionError{"Le nom d'utilisateur est requis"}
	errMissingAuth     = &connectionError{"Le mot de passe ou la clé privée est requis"}
	errInvalidPort     = &connectionError{"Numéro de port invalide (1-65535)"}
)

type connectionError struct {
	message string
}

func (e *connectionError) Error() string {
	return e.message
}

func (cd *ConnectionDialog) deleteSelectedProfile() {
	if cd.selectedProfileID == "" {
		return
	}

	profiles := cd.configMgr.GetProfiles()
	var profileName string
	for _, p := range profiles {
		if p.ID == cd.selectedProfileID {
			profileName = p.Name
			break
		}
	}

	dialog.ShowConfirm("Supprimer le profil",
		fmt.Sprintf("Supprimer le profil '%s' ?", profileName),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			if cd.credentialsMgr != nil {
				cd.credentialsMgr.DeletePassword(cd.selectedProfileID)
			}

			cd.configMgr.DeleteProfile(cd.selectedProfileID)

			cd.refreshProfileList()
			cd.clearForm()
			cd.profileSelect.SetSelectedIndex(0)
			cd.deleteProfileBtn.Disable()
		}, cd.window)
}

func (cd *ConnectionDialog) refreshProfileList() {
	profiles := cd.configMgr.GetProfiles()
	profileNames := make([]string, len(profiles)+1)
	profileNames[0] = "-- Nouvelle connexion --"
	for i, p := range profiles {
		profileNames[i+1] = p.Name
	}
	cd.profileSelect.Options = profileNames
	cd.profileSelect.Refresh()
}
