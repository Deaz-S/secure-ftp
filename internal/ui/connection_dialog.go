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

type ConnectionDialog struct {
	window         fyne.Window
	configMgr      *config.ConfigManager
	credentialsMgr *config.CredentialsManager
	onConnect      func(*config.ConnectionProfile, string)

	selectedProfileID string

	profileSelect     *widget.Select
	deleteProfileBtn  *widget.Button
	protocolSelect    *widget.Select
	hostEntry         *widget.Entry
	portEntry         *widget.Entry
	usernameEntry     *widget.Entry
	passwordEntry     *widget.Entry
	privateKeyEntry   *widget.Entry
	privateKeyBtn     *widget.Button
	privateKeyLabel   *widget.Label
	privateKeyRow     *fyne.Container
	remoteDirEntry    *widget.Entry
	tlsImplicitCheck  *widget.Check
	saveProfileCheck  *widget.Check
	savePasswordCheck *widget.Check
	profileNameEntry  *widget.Entry
}

func NewConnectionDialog(parent fyne.Window, configMgr *config.ConfigManager, credsMgr *config.CredentialsManager, onConnect func(*config.ConnectionProfile, string)) *ConnectionDialog {
	return &ConnectionDialog{
		window:         parent,
		configMgr:      configMgr,
		credentialsMgr: credsMgr,
		onConnect:      onConnect,
	}
}

func (cd *ConnectionDialog) Show() {
	cd.buildForm()
}

func (cd *ConnectionDialog) buildForm() {
	cd.hostEntry = widget.NewEntry()
	cd.hostEntry.SetPlaceHolder("nom d'hôte ou adresse IP")

	cd.portEntry = widget.NewEntry()
	cd.portEntry.SetPlaceHolder("22")
	cd.portEntry.SetText("22")

	cd.usernameEntry = widget.NewEntry()
	cd.usernameEntry.SetPlaceHolder("nom d'utilisateur")

	cd.passwordEntry = widget.NewPasswordEntry()
	cd.passwordEntry.SetPlaceHolder("mot de passe")

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

	cd.privateKeyLabel = widget.NewLabel("Clé privée (SSH) :")
	cd.privateKeyRow = container.NewBorder(nil, nil, nil, cd.privateKeyBtn, cd.privateKeyEntry)

	cd.remoteDirEntry = widget.NewEntry()
	cd.remoteDirEntry.SetPlaceHolder("/home/user (optionnel)")

	cd.tlsImplicitCheck = widget.NewCheck("TLS implicite (port 990)", nil)
	cd.tlsImplicitCheck.Hide()

	cd.saveProfileCheck = widget.NewCheck("Enregistrer comme profil", nil)
	cd.profileNameEntry = widget.NewEntry()
	cd.profileNameEntry.SetPlaceHolder("Nom du profil")
	cd.profileNameEntry.Hide()

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

	cd.protocolSelect = widget.NewSelect([]string{"SFTP", "FTPS", "FTP"}, cd.onProtocolSelected)

	profiles := cd.configMgr.GetProfiles()
	profileNames := make([]string, len(profiles)+1)
	profileNames[0] = "-- Nouvelle connexion --"
	for i, p := range profiles {
		profileNames[i+1] = p.Name
	}
	cd.profileSelect = widget.NewSelect(profileNames, cd.onProfileSelected)

	cd.deleteProfileBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		fmt.Println("DEBUG: Delete button clicked, selectedProfileID =", cd.selectedProfileID)
		cd.deleteSelectedProfile()
	})
	cd.deleteProfileBtn.Importance = widget.DangerImportance
	cd.deleteProfileBtn.Disable()
	fmt.Println("DEBUG: Delete button created")

	cd.protocolSelect.SetSelectedIndex(0)
	cd.profileSelect.SetSelectedIndex(0)

	profileRow := container.NewHBox(cd.profileSelect, cd.deleteProfileBtn)

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
		cd.privateKeyLabel,
		cd.privateKeyRow,
		widget.NewSeparator(),
		widget.NewLabel("Répertoire distant :"),
		cd.remoteDirEntry,
		widget.NewSeparator(),
		cd.saveProfileCheck,
		cd.profileNameEntry,
		cd.savePasswordCheck,
	)

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(380, 500))

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
	fmt.Println("DEBUG: onProfileSelected called with:", selected)
	if selected == "-- Nouvelle connexion --" {
		cd.clearForm()
		cd.selectedProfileID = ""
		cd.deleteProfileBtn.Disable()
		fmt.Println("DEBUG: Nouvelle connexion selected, button disabled")
		return
	}

	profiles := cd.configMgr.GetProfiles()
	fmt.Printf("DEBUG: Found %d profiles\n", len(profiles))
	for _, p := range profiles {
		if p.Name == selected {
			cd.loadProfile(&p)
			cd.deleteProfileBtn.Enable()
			fmt.Println("DEBUG: Profile loaded, delete button enabled")
			return
		}
	}
}

func (cd *ConnectionDialog) loadProfile(profile *config.ConnectionProfile) {
	fmt.Printf("DEBUG: loadProfile called, profile.ID=%s, profile.Name=%s\n", profile.ID, profile.Name)
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

	if cd.credentialsMgr != nil && profile.ID != "" {
		if password, err := cd.credentialsMgr.GetPassword(profile.ID); err == nil && password != "" {
			cd.passwordEntry.SetText(password)
		} else {
			cd.passwordEntry.SetText("")
		}
	} else {
		cd.passwordEntry.SetText("")
	}

	cd.saveProfileCheck.SetChecked(false)
	cd.saveProfileCheck.Hide()
	cd.profileNameEntry.Hide()
	cd.savePasswordCheck.Hide()

	cd.onProtocolSelected(cd.protocolSelect.Selected)
}

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
	cd.onProtocolSelected("SFTP")
}

func (cd *ConnectionDialog) onProtocolSelected(selected string) {
	switch selected {
	case "SFTP":
		cd.portEntry.SetText("22")
		cd.tlsImplicitCheck.Hide()
		cd.privateKeyLabel.Show()
		cd.privateKeyRow.Show()
	case "FTPS":
		if cd.tlsImplicitCheck.Checked {
			cd.portEntry.SetText("990")
		} else {
			cd.portEntry.SetText("21")
		}
		cd.tlsImplicitCheck.Show()
		cd.privateKeyLabel.Hide()
		cd.privateKeyRow.Hide()
	case "FTP":
		cd.portEntry.SetText("21")
		cd.tlsImplicitCheck.Hide()
		cd.privateKeyLabel.Hide()
		cd.privateKeyRow.Hide()
	}
}

func (cd *ConnectionDialog) handleConnect() {
	if cd.hostEntry.Text == "" {
		dialog.ShowError(errMissingHost, cd.window)
		return
	}

	if cd.usernameEntry.Text == "" {
		dialog.ShowError(errMissingUsername, cd.window)
		return
	}

	if cd.passwordEntry.Text == "" && cd.privateKeyEntry.Text == "" {
		dialog.ShowError(errMissingAuth, cd.window)
		return
	}

	port, err := strconv.Atoi(cd.portEntry.Text)
	if err != nil || port < 1 || port > 65535 {
		dialog.ShowError(errInvalidPort, cd.window)
		return
	}

	protocol := "sftp"
	switch cd.protocolSelect.Selected {
	case "FTPS":
		protocol = "ftps"
	case "FTP":
		protocol = "ftp"
	}

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

	if cd.saveProfileCheck.Checked && cd.profileNameEntry.Text != "" {
		if err := cd.configMgr.AddProfile(*profile); err == nil {
			profiles := cd.configMgr.GetProfiles()
			for _, p := range profiles {
				if p.Name == profile.Name {
					profile.ID = p.ID
					break
				}
			}

			if cd.savePasswordCheck.Checked && cd.credentialsMgr != nil && profile.ID != "" {
				cd.credentialsMgr.SetPassword(profile.ID, cd.passwordEntry.Text)
			}
		}
	}

	if cd.onConnect != nil {
		cd.onConnect(profile, cd.passwordEntry.Text)
	}
}

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
	fmt.Println("DEBUG: deleteSelectedProfile called, selectedProfileID =", cd.selectedProfileID)
	if cd.selectedProfileID == "" {
		fmt.Println("DEBUG: selectedProfileID is empty, returning")
		return
	}
	fmt.Println("DEBUG: Proceeding with deletion")

	if cd.credentialsMgr != nil {
		fmt.Println("DEBUG: Deleting password...")
		cd.credentialsMgr.DeletePassword(cd.selectedProfileID)
	}

	fmt.Println("DEBUG: Calling configMgr.DeleteProfile...")
	err := cd.configMgr.DeleteProfile(cd.selectedProfileID)
	if err != nil {
		fmt.Println("DEBUG: DeleteProfile error:", err)
	} else {
		fmt.Println("DEBUG: DeleteProfile success")
	}
	cd.selectedProfileID = ""

	fmt.Println("DEBUG: Refreshing profile list...")
	cd.refreshProfileList()
	fmt.Println("DEBUG: Clearing form...")
	cd.clearForm()
	fmt.Println("DEBUG: Setting select index to 0...")
	cd.profileSelect.SetSelectedIndex(0)
	fmt.Println("DEBUG: Disabling delete button...")
	cd.deleteProfileBtn.Disable()
	fmt.Println("DEBUG: deleteSelectedProfile complete")
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
