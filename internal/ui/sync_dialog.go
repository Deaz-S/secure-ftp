// Package ui provides the sync dialog.
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	ftpsync "secure-ftp/internal/sync"
)

// SyncDialog handles folder synchronization configuration.
type SyncDialog struct {
	window    fyne.Window
	localDir  string
	remoteDir string
	onSync    func(options ftpsync.SyncOptions, localDir, remoteDir string)

	// UI components
	modeSelect        *widget.Select
	comparisonSelect  *widget.Select
	deleteExtra       *widget.Check
	ignoreHidden      *widget.Check
	dryRun            *widget.Check
	excludePatterns   *widget.Entry
	includePatterns   *widget.Entry
}

// NewSyncDialog creates a new sync dialog.
func NewSyncDialog(parent fyne.Window, localDir, remoteDir string, onSync func(options ftpsync.SyncOptions, localDir, remoteDir string)) *SyncDialog {
	return &SyncDialog{
		window:    parent,
		localDir:  localDir,
		remoteDir: remoteDir,
		onSync:    onSync,
	}
}

// Show displays the sync dialog.
func (sd *SyncDialog) Show() {
	sd.buildDialog()
}

// buildDialog constructs the dialog.
func (sd *SyncDialog) buildDialog() {
	// Sync mode selection
	sd.modeSelect = widget.NewSelect([]string{
		"Envoi (Local → Distant)",
		"Téléchargement (Distant → Local)",
		"Miroir (Distant = Local)",
		"Bidirectionnel (Fusionner)",
	}, nil)
	sd.modeSelect.SetSelectedIndex(0)

	// Comparison method
	sd.comparisonSelect = widget.NewSelect([]string{
		"Taille + Date de modification",
		"Taille uniquement",
		"Date de modification uniquement",
		"Somme de contrôle (MD5)",
	}, nil)
	sd.comparisonSelect.SetSelectedIndex(0)

	// Options
	sd.deleteExtra = widget.NewCheck("Supprimer les fichiers supplémentaires à la destination", nil)
	sd.ignoreHidden = widget.NewCheck("Ignorer les fichiers cachés", nil)
	sd.ignoreHidden.SetChecked(true)
	sd.dryRun = widget.NewCheck("Simulation (aperçu uniquement)", nil)

	// Patterns
	sd.excludePatterns = widget.NewEntry()
	sd.excludePatterns.SetPlaceHolder("*.tmp, *.log, .git/")
	sd.excludePatterns.MultiLine = true

	sd.includePatterns = widget.NewEntry()
	sd.includePatterns.SetPlaceHolder("*.go, *.js, *.py")
	sd.includePatterns.MultiLine = true

	// Summary
	summaryLabel := widget.NewLabel(fmt.Sprintf(
		"Local : %s\nDistant : %s",
		sd.localDir,
		sd.remoteDir,
	))
	summaryLabel.Wrapping = fyne.TextWrapWord

	// Form layout
	form := container.NewVBox(
		widget.NewLabel("Dossiers"),
		widget.NewSeparator(),
		summaryLabel,

		widget.NewLabel(""),
		widget.NewLabel("Mode de synchronisation"),
		widget.NewSeparator(),
		sd.modeSelect,

		widget.NewLabel(""),
		widget.NewLabel("Méthode de comparaison"),
		widget.NewSeparator(),
		sd.comparisonSelect,

		widget.NewLabel(""),
		widget.NewLabel("Options"),
		widget.NewSeparator(),
		sd.deleteExtra,
		sd.ignoreHidden,
		sd.dryRun,

		widget.NewLabel(""),
		widget.NewLabel("Motifs d'exclusion (séparés par des virgules)"),
		widget.NewSeparator(),
		sd.excludePatterns,

		widget.NewLabel(""),
		widget.NewLabel("Motifs d'inclusion (séparés par des virgules, optionnel)"),
		widget.NewSeparator(),
		sd.includePatterns,
	)

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(400, 450))

	dlg := dialog.NewCustomConfirm("Synchronisation des dossiers", "Démarrer", "Annuler", scroll,
		func(confirmed bool) {
			if confirmed {
				sd.startSync()
			}
		}, sd.window)

	dlg.Resize(fyne.NewSize(500, 550))
	dlg.Show()
}

// startSync starts the synchronization with the configured options.
func (sd *SyncDialog) startSync() {
	// Parse sync mode
	var mode ftpsync.SyncMode
	switch sd.modeSelect.SelectedIndex() {
	case 0:
		mode = ftpsync.ModeUpload
	case 1:
		mode = ftpsync.ModeDownload
	case 2:
		mode = ftpsync.ModeMirror
	case 3:
		mode = ftpsync.ModeBidirectional
	}

	// Parse comparison method
	var comparison ftpsync.CompareMethod
	switch sd.comparisonSelect.SelectedIndex() {
	case 0:
		comparison = ftpsync.CompareBySizeAndTime
	case 1:
		comparison = ftpsync.CompareBySize
	case 2:
		comparison = ftpsync.CompareByModTime
	case 3:
		comparison = ftpsync.CompareByHash
	}

	// Build options
	options := ftpsync.SyncOptions{
		Mode:            mode,
		CompareMethod:   comparison,
		DeleteExtra:     sd.deleteExtra.Checked,
		IgnoreHidden:    sd.ignoreHidden.Checked,
		DryRun:          sd.dryRun.Checked,
		ExcludePatterns: parsePatterns(sd.excludePatterns.Text),
		IncludePatterns: parsePatterns(sd.includePatterns.Text),
	}

	if sd.onSync != nil {
		sd.onSync(options, sd.localDir, sd.remoteDir)
	}
}

// parsePatterns parses a comma-separated pattern string.
func parsePatterns(text string) []string {
	if text == "" {
		return nil
	}

	var patterns []string
	for _, p := range splitAndTrim(text, ",") {
		if p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// splitAndTrim splits a string and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	current := ""
	for _, c := range s {
		if string(c) == sep {
			trimmed := trimSpace(current)
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	trimmed := trimSpace(current)
	if trimmed != "" {
		parts = append(parts, trimmed)
	}
	return parts
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}
