// Package ui provides the profiles management dialog.
package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/config"
)

// ProfilesDialog handles profile management.
type ProfilesDialog struct {
	window         fyne.Window
	configMgr      *config.ConfigManager
	credentialsMgr *config.CredentialsManager
	onUpdate       func()

	// UI components
	profileList   *widget.List
	profiles      []config.ConnectionProfile
	selectedIndex int

	// Edit fields
	nameEntry       *widget.Entry
	protocolSelect  *widget.Select
	hostEntry       *widget.Entry
	portEntry       *widget.Entry
	usernameEntry   *widget.Entry
	privateKeyEntry *widget.Entry
	remoteDirEntry  *widget.Entry
	tlsImplicitCheck *widget.Check
}

// NewProfilesDialog creates a new profiles management dialog.
func NewProfilesDialog(parent fyne.Window, configMgr *config.ConfigManager, credsMgr *config.CredentialsManager, onUpdate func()) *ProfilesDialog {
	return &ProfilesDialog{
		window:         parent,
		configMgr:      configMgr,
		credentialsMgr: credsMgr,
		onUpdate:       onUpdate,
		selectedIndex:  -1,
	}
}

// Show displays the profiles dialog.
func (pd *ProfilesDialog) Show() {
	pd.profiles = pd.configMgr.GetProfiles()
	pd.buildDialog()
}

// buildDialog constructs the dialog.
func (pd *ProfilesDialog) buildDialog() {
	// Create profile list
	pd.profileList = widget.NewList(
		func() int {
			return len(pd.profiles)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(nil),
				widget.NewLabel("Profile Name"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			label := box.Objects[1].(*widget.Label)
			profile := pd.profiles[id]
			label.SetText(fmt.Sprintf("%s (%s@%s)", profile.Name, profile.Username, profile.Host))
		},
	)

	pd.profileList.OnSelected = func(id widget.ListItemID) {
		pd.selectedIndex = id
		pd.loadProfileToForm(id)
	}

	// Create edit form
	pd.nameEntry = widget.NewEntry()
	pd.nameEntry.SetPlaceHolder("Nom du profil")

	pd.protocolSelect = widget.NewSelect([]string{"SFTP", "FTPS", "FTP"}, nil)

	pd.hostEntry = widget.NewEntry()
	pd.hostEntry.SetPlaceHolder("nom d'hôte")

	pd.portEntry = widget.NewEntry()
	pd.portEntry.SetPlaceHolder("22")

	pd.usernameEntry = widget.NewEntry()
	pd.usernameEntry.SetPlaceHolder("nom d'utilisateur")

	pd.privateKeyEntry = widget.NewEntry()
	pd.privateKeyEntry.SetPlaceHolder("~/.ssh/id_rsa")

	pd.remoteDirEntry = widget.NewEntry()
	pd.remoteDirEntry.SetPlaceHolder("/home/utilisateur")

	pd.tlsImplicitCheck = widget.NewCheck("TLS implicite", nil)

	// Buttons
	saveBtn := widget.NewButton("Enregistrer", pd.saveProfile)
	deleteBtn := widget.NewButton("Supprimer", pd.deleteProfile)
	clearPwdBtn := widget.NewButton("Effacer mot de passe", pd.clearPassword)

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Nom :"),
		pd.nameEntry,
		widget.NewLabel("Protocole :"),
		pd.protocolSelect,
		widget.NewLabel("Hôte :"),
		pd.hostEntry,
		widget.NewLabel("Port :"),
		pd.portEntry,
		widget.NewLabel("Nom d'utilisateur :"),
		pd.usernameEntry,
		widget.NewLabel("Clé privée :"),
		pd.privateKeyEntry,
		widget.NewLabel("Répertoire distant :"),
		pd.remoteDirEntry,
		pd.tlsImplicitCheck,
		widget.NewSeparator(),
		container.NewHBox(saveBtn, deleteBtn, clearPwdBtn),
	)

	formScroll := container.NewVScroll(form)
	formScroll.SetMinSize(fyne.NewSize(300, 400))

	// List panel
	listPanel := container.NewBorder(
		widget.NewLabel("Profils enregistrés"),
		nil, nil, nil,
		pd.profileList,
	)

	// Main layout
	split := container.NewHSplit(listPanel, formScroll)
	split.SetOffset(0.4)

	dlg := dialog.NewCustom("Gestion des profils", "Fermer", split, pd.window)
	dlg.Resize(fyne.NewSize(700, 500))
	dlg.Show()
}

// loadProfileToForm loads a profile into the edit form.
func (pd *ProfilesDialog) loadProfileToForm(index int) {
	if index < 0 || index >= len(pd.profiles) {
		return
	}

	profile := pd.profiles[index]
	pd.nameEntry.SetText(profile.Name)
	pd.hostEntry.SetText(profile.Host)
	pd.portEntry.SetText(strconv.Itoa(profile.Port))
	pd.usernameEntry.SetText(profile.Username)
	pd.privateKeyEntry.SetText(profile.PrivateKeyPath)
	pd.remoteDirEntry.SetText(profile.RemoteDir)
	pd.tlsImplicitCheck.SetChecked(profile.TLSImplicit)

	switch profile.Protocol {
	case "sftp":
		pd.protocolSelect.SetSelectedIndex(0)
	case "ftps":
		pd.protocolSelect.SetSelectedIndex(1)
	case "ftp":
		pd.protocolSelect.SetSelectedIndex(2)
	}
}

// saveProfile saves the current profile.
func (pd *ProfilesDialog) saveProfile() {
	if pd.selectedIndex < 0 || pd.selectedIndex >= len(pd.profiles) {
		dialog.ShowError(fmt.Errorf("Aucun profil sélectionné"), pd.window)
		return
	}

	port, _ := strconv.Atoi(pd.portEntry.Text)
	if port < 1 || port > 65535 {
		dialog.ShowError(fmt.Errorf("Numéro de port invalide"), pd.window)
		return
	}

	protocol := "sftp"
	switch pd.protocolSelect.Selected {
	case "FTPS":
		protocol = "ftps"
	case "FTP":
		protocol = "ftp"
	}

	profile := config.ConnectionProfile{
		ID:             pd.profiles[pd.selectedIndex].ID,
		Name:           pd.nameEntry.Text,
		Protocol:       protocol,
		Host:           pd.hostEntry.Text,
		Port:           port,
		Username:       pd.usernameEntry.Text,
		PrivateKeyPath: pd.privateKeyEntry.Text,
		RemoteDir:      pd.remoteDirEntry.Text,
		TLSImplicit:    pd.tlsImplicitCheck.Checked,
		LastUsed:       pd.profiles[pd.selectedIndex].LastUsed,
	}

	if err := pd.configMgr.UpdateProfile(profile); err != nil {
		dialog.ShowError(err, pd.window)
		return
	}

	pd.profiles = pd.configMgr.GetProfiles()
	pd.profileList.Refresh()

	if pd.onUpdate != nil {
		pd.onUpdate()
	}

	dialog.ShowInformation("Succès", "Profil enregistré", pd.window)
}

// deleteProfile deletes the selected profile.
func (pd *ProfilesDialog) deleteProfile() {
	if pd.selectedIndex < 0 || pd.selectedIndex >= len(pd.profiles) {
		dialog.ShowError(fmt.Errorf("Aucun profil sélectionné"), pd.window)
		return
	}

	profile := pd.profiles[pd.selectedIndex]

	dialog.ShowConfirm("Supprimer le profil",
		fmt.Sprintf("Êtes-vous sûr de vouloir supprimer '%s' ?", profile.Name),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			// Delete password if stored
			if pd.credentialsMgr != nil {
				pd.credentialsMgr.DeletePassword(profile.ID)
			}

			// Delete profile
			if err := pd.configMgr.DeleteProfile(profile.ID); err != nil {
				dialog.ShowError(err, pd.window)
				return
			}

			pd.profiles = pd.configMgr.GetProfiles()
			pd.selectedIndex = -1
			pd.profileList.Refresh()
			pd.clearForm()

			if pd.onUpdate != nil {
				pd.onUpdate()
			}
		}, pd.window)
}

// clearPassword clears the stored password for the selected profile.
func (pd *ProfilesDialog) clearPassword() {
	if pd.selectedIndex < 0 || pd.selectedIndex >= len(pd.profiles) {
		dialog.ShowError(fmt.Errorf("Aucun profil sélectionné"), pd.window)
		return
	}

	if pd.credentialsMgr == nil {
		dialog.ShowError(fmt.Errorf("Gestionnaire d'identifiants non disponible"), pd.window)
		return
	}

	profile := pd.profiles[pd.selectedIndex]
	if err := pd.credentialsMgr.DeletePassword(profile.ID); err != nil {
		dialog.ShowError(err, pd.window)
		return
	}

	dialog.ShowInformation("Succès", "Mot de passe enregistré effacé", pd.window)
}

// clearForm clears the edit form.
func (pd *ProfilesDialog) clearForm() {
	pd.nameEntry.SetText("")
	pd.hostEntry.SetText("")
	pd.portEntry.SetText("")
	pd.usernameEntry.SetText("")
	pd.privateKeyEntry.SetText("")
	pd.remoteDirEntry.SetText("")
	pd.tlsImplicitCheck.SetChecked(false)
	pd.protocolSelect.ClearSelected()
}
