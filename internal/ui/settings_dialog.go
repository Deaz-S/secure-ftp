// Package ui provides the settings dialog.
package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/config"
	"secure-ftp/internal/transfer"
)

// SettingsDialog handles application settings.
type SettingsDialog struct {
	window    fyne.Window
	configMgr *config.ConfigManager
	onSave    func()

	// UI components
	themeSelect          *widget.Select
	parallelTransfers    *widget.Entry
	showHiddenFiles      *widget.Check
	defaultLocalDir      *widget.Entry
	logLevelSelect       *widget.Select
	windowWidth          *widget.Entry
	windowHeight         *widget.Entry
	uploadRateSelect     *widget.Select
	downloadRateSelect   *widget.Select
	enableNotifications  *widget.Check
}

// NewSettingsDialog creates a new settings dialog.
func NewSettingsDialog(parent fyne.Window, configMgr *config.ConfigManager, onSave func()) *SettingsDialog {
	return &SettingsDialog{
		window:    parent,
		configMgr: configMgr,
		onSave:    onSave,
	}
}

// Show displays the settings dialog.
func (sd *SettingsDialog) Show() {
	sd.buildDialog()
}

// buildDialog constructs the dialog.
func (sd *SettingsDialog) buildDialog() {
	cfg := sd.configMgr.Get()

	// Theme selection
	sd.themeSelect = widget.NewSelect([]string{"système", "clair", "sombre"}, nil)
	sd.themeSelect.SetSelected(cfg.Theme)

	// Parallel transfers
	sd.parallelTransfers = widget.NewEntry()
	sd.parallelTransfers.SetText(strconv.Itoa(cfg.MaxParallelTransfers))

	// Bandwidth presets
	presets := transfer.GetBandwidthPresets()
	presetNames := make([]string, len(presets))
	for i, p := range presets {
		presetNames[i] = p.Name
	}

	// Upload rate limit
	sd.uploadRateSelect = widget.NewSelect(presetNames, nil)
	sd.uploadRateSelect.SetSelected(sd.rateToPresetName(cfg.UploadRateLimit))

	// Download rate limit
	sd.downloadRateSelect = widget.NewSelect(presetNames, nil)
	sd.downloadRateSelect.SetSelected(sd.rateToPresetName(cfg.DownloadRateLimit))

	// Show hidden files
	sd.showHiddenFiles = widget.NewCheck("", nil)
	sd.showHiddenFiles.SetChecked(cfg.ShowHiddenFiles)

	// Enable notifications
	sd.enableNotifications = widget.NewCheck("", nil)
	sd.enableNotifications.SetChecked(cfg.EnableNotifications)

	// Default local directory
	sd.defaultLocalDir = widget.NewEntry()
	sd.defaultLocalDir.SetText(cfg.DefaultLocalDir)

	browseDirBtn := widget.NewButton("Parcourir...", func() {
		dlg := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			sd.defaultLocalDir.SetText(uri.Path())
		}, sd.window)
		dlg.Show()
	})

	dirRow := container.NewBorder(nil, nil, nil, browseDirBtn, sd.defaultLocalDir)

	// Log level
	sd.logLevelSelect = widget.NewSelect([]string{"debug", "info", "warn", "error"}, nil)
	sd.logLevelSelect.SetSelected(cfg.LogLevel)

	// Window size
	sd.windowWidth = widget.NewEntry()
	sd.windowWidth.SetText(strconv.Itoa(cfg.WindowWidth))

	sd.windowHeight = widget.NewEntry()
	sd.windowHeight.SetText(strconv.Itoa(cfg.WindowHeight))

	windowSizeRow := container.NewHBox(
		sd.windowWidth,
		widget.NewLabel("x"),
		sd.windowHeight,
	)

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Apparence"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Thème :"),
			sd.themeSelect,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Taille de fenêtre :"),
			windowSizeRow,
		),

		widget.NewLabel(""),
		widget.NewLabel("Transferts"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Transferts parallèles max :"),
			sd.parallelTransfers,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Limite vitesse envoi :"),
			sd.uploadRateSelect,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Limite vitesse téléchargement :"),
			sd.downloadRateSelect,
		),

		widget.NewLabel(""),
		widget.NewLabel("Navigateur de fichiers"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Afficher fichiers cachés :"),
			sd.showHiddenFiles,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Répertoire local par défaut :"),
			dirRow,
		),

		widget.NewLabel(""),
		widget.NewLabel("Notifications"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Notifications bureau :"),
			sd.enableNotifications,
		),

		widget.NewLabel(""),
		widget.NewLabel("Journalisation"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Niveau de log :"),
			sd.logLevelSelect,
		),
	)

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(400, 400))

	dlg := dialog.NewCustomConfirm("Paramètres", "Enregistrer", "Annuler", scroll,
		func(confirmed bool) {
			if confirmed {
				sd.saveSettings()
			}
		}, sd.window)

	dlg.Resize(fyne.NewSize(500, 500))
	dlg.Show()
}

// rateToPresetName converts a rate in bytes/sec to preset name.
func (sd *SettingsDialog) rateToPresetName(rate int64) string {
	presets := transfer.GetBandwidthPresets()
	for _, p := range presets {
		if p.BytesPerSecond == rate {
			return p.Name
		}
	}
	return "Unlimited"
}

// presetNameToRate converts a preset name to rate in bytes/sec.
func (sd *SettingsDialog) presetNameToRate(name string) int64 {
	presets := transfer.GetBandwidthPresets()
	for _, p := range presets {
		if p.Name == name {
			return p.BytesPerSecond
		}
	}
	return 0
}

// saveSettings saves the current settings.
func (sd *SettingsDialog) saveSettings() {
	cfg := sd.configMgr.Get()

	// Parse and validate values
	parallelTransfers, err := strconv.Atoi(sd.parallelTransfers.Text)
	if err != nil || parallelTransfers < 1 || parallelTransfers > 10 {
		dialog.ShowError(&settingsError{"Les transferts parallèles doivent être entre 1 et 10"}, sd.window)
		return
	}

	windowWidth, err := strconv.Atoi(sd.windowWidth.Text)
	if err != nil || windowWidth < 400 {
		dialog.ShowError(&settingsError{"La largeur de fenêtre doit être au moins 400"}, sd.window)
		return
	}

	windowHeight, err := strconv.Atoi(sd.windowHeight.Text)
	if err != nil || windowHeight < 300 {
		dialog.ShowError(&settingsError{"La hauteur de fenêtre doit être au moins 300"}, sd.window)
		return
	}

	// Update config
	cfg.Theme = sd.themeSelect.Selected
	cfg.MaxParallelTransfers = parallelTransfers
	cfg.ShowHiddenFiles = sd.showHiddenFiles.Checked
	cfg.DefaultLocalDir = sd.defaultLocalDir.Text
	cfg.LogLevel = sd.logLevelSelect.Selected
	cfg.WindowWidth = windowWidth
	cfg.WindowHeight = windowHeight
	cfg.UploadRateLimit = sd.presetNameToRate(sd.uploadRateSelect.Selected)
	cfg.DownloadRateLimit = sd.presetNameToRate(sd.downloadRateSelect.Selected)
	cfg.EnableNotifications = sd.enableNotifications.Checked

	if err := sd.configMgr.Set(&cfg); err != nil {
		dialog.ShowError(err, sd.window)
		return
	}

	if sd.onSave != nil {
		sd.onSave()
	}

	dialog.ShowInformation("Paramètres", "Paramètres enregistrés. Certaines modifications nécessitent un redémarrage.", sd.window)
}

type settingsError struct {
	message string
}

func (e *settingsError) Error() string {
	return e.message
}
