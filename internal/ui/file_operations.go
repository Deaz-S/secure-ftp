// Package ui provides file operation dialogs and actions.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/protocol"
)

// FileOperations handles file management operations.
type FileOperations struct {
	window fyne.Window
	client protocol.Protocol
}

// NewFileOperations creates a new file operations handler.
func NewFileOperations(window fyne.Window) *FileOperations {
	return &FileOperations{
		window: window,
	}
}

// SetClient sets the protocol client for remote operations.
func (fo *FileOperations) SetClient(client protocol.Protocol) {
	fo.client = client
}

// RenameLocal renames a local file or directory.
func (fo *FileOperations) RenameLocal(path string, onComplete func()) {
	oldName := filepath.Base(path)
	entry := widget.NewEntry()
	entry.SetText(oldName)

	dialog.ShowForm("Renommer", "Renommer", "Annuler",
		[]*widget.FormItem{
			widget.NewFormItem("Nouveau nom :", entry),
		},
		func(confirmed bool) {
			if !confirmed || entry.Text == "" || entry.Text == oldName {
				return
			}

			newPath := filepath.Join(filepath.Dir(path), entry.Text)
			if err := os.Rename(path, newPath); err != nil {
				dialog.ShowError(fmt.Errorf("échec du renommage : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// RenameRemote renames a remote file or directory.
func (fo *FileOperations) RenameRemote(path string, onComplete func()) {
	if fo.client == nil {
		dialog.ShowError(fmt.Errorf("non connecté"), fo.window)
		return
	}

	oldName := filepath.Base(path)
	entry := widget.NewEntry()
	entry.SetText(oldName)

	dialog.ShowForm("Renommer", "Renommer", "Annuler",
		[]*widget.FormItem{
			widget.NewFormItem("Nouveau nom :", entry),
		},
		func(confirmed bool) {
			if !confirmed || entry.Text == "" || entry.Text == oldName {
				return
			}

			newPath := filepath.Join(filepath.Dir(path), entry.Text)
			if err := fo.client.Rename(context.Background(), path, newPath); err != nil {
				dialog.ShowError(fmt.Errorf("échec du renommage : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// DeleteLocal deletes a local file or directory.
func (fo *FileOperations) DeleteLocal(path string, isDir bool, onComplete func()) {
	itemType := "le fichier"
	if isDir {
		itemType = "le dossier"
	}

	dialog.ShowConfirm("Supprimer "+itemType,
		fmt.Sprintf("Êtes-vous sûr de vouloir supprimer '%s' ?", filepath.Base(path)),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			var err error
			if isDir {
				err = os.RemoveAll(path)
			} else {
				err = os.Remove(path)
			}

			if err != nil {
				dialog.ShowError(fmt.Errorf("échec de la suppression : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// DeleteRemote deletes a remote file or directory.
func (fo *FileOperations) DeleteRemote(path string, isDir bool, onComplete func()) {
	if fo.client == nil {
		dialog.ShowError(fmt.Errorf("non connecté"), fo.window)
		return
	}

	itemType := "le fichier"
	if isDir {
		itemType = "le dossier"
	}

	dialog.ShowConfirm("Supprimer "+itemType,
		fmt.Sprintf("Êtes-vous sûr de vouloir supprimer '%s' ?", filepath.Base(path)),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			var err error
			if isDir {
				err = fo.client.RemoveDir(context.Background(), path)
			} else {
				err = fo.client.Remove(context.Background(), path)
			}

			if err != nil {
				dialog.ShowError(fmt.Errorf("échec de la suppression : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// CreateFolderLocal creates a new local folder.
func (fo *FileOperations) CreateFolderLocal(parentPath string, onComplete func()) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Nouveau dossier")

	dialog.ShowForm("Nouveau dossier", "Créer", "Annuler",
		[]*widget.FormItem{
			widget.NewFormItem("Nom du dossier :", entry),
		},
		func(confirmed bool) {
			if !confirmed || entry.Text == "" {
				return
			}

			newPath := filepath.Join(parentPath, entry.Text)
			if err := os.MkdirAll(newPath, 0755); err != nil {
				dialog.ShowError(fmt.Errorf("échec de la création du dossier : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// CreateFolderRemote creates a new remote folder.
func (fo *FileOperations) CreateFolderRemote(parentPath string, onComplete func()) {
	if fo.client == nil {
		dialog.ShowError(fmt.Errorf("non connecté"), fo.window)
		return
	}

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Nouveau dossier")

	dialog.ShowForm("Nouveau dossier", "Créer", "Annuler",
		[]*widget.FormItem{
			widget.NewFormItem("Nom du dossier :", entry),
		},
		func(confirmed bool) {
			if !confirmed || entry.Text == "" {
				return
			}

			newPath := filepath.Join(parentPath, entry.Text)
			if err := fo.client.Mkdir(context.Background(), newPath); err != nil {
				dialog.ShowError(fmt.Errorf("échec de la création du dossier : %v", err), fo.window)
				return
			}

			if onComplete != nil {
				onComplete()
			}
		},
		fo.window,
	)
}

// ShowProperties shows file/directory properties.
func (fo *FileOperations) ShowPropertiesLocal(path string) {
	info, err := os.Stat(path)
	if err != nil {
		dialog.ShowError(err, fo.window)
		return
	}

	var content string
	if info.IsDir() {
		// Count items in directory
		entries, _ := os.ReadDir(path)
		content = fmt.Sprintf(
			"Nom : %s\nType : Dossier\nÉléments : %d\nChemin : %s\nPermissions : %s\nModifié : %s",
			info.Name(),
			len(entries),
			path,
			info.Mode().String(),
			info.ModTime().Format("02/01/2006 15:04:05"),
		)
	} else {
		content = fmt.Sprintf(
			"Nom : %s\nType : Fichier\nTaille : %s\nChemin : %s\nPermissions : %s\nModifié : %s",
			info.Name(),
			formatFileSize(info.Size()),
			path,
			info.Mode().String(),
			info.ModTime().Format("02/01/2006 15:04:05"),
		)
	}

	dialog.ShowInformation("Propriétés", content, fo.window)
}

// ShowPropertiesRemote shows remote file/directory properties.
func (fo *FileOperations) ShowPropertiesRemote(path string) {
	if fo.client == nil {
		dialog.ShowError(fmt.Errorf("non connecté"), fo.window)
		return
	}

	info, err := fo.client.Stat(context.Background(), path)
	if err != nil {
		dialog.ShowError(err, fo.window)
		return
	}

	var content string
	if info.IsDir {
		content = fmt.Sprintf(
			"Nom : %s\nType : Dossier\nChemin : %s\nPermissions : %s\nModifié : %s",
			info.Name,
			path,
			info.Permissions,
			info.ModTime.Format("02/01/2006 15:04:05"),
		)
	} else {
		content = fmt.Sprintf(
			"Nom : %s\nType : Fichier\nTaille : %s\nChemin : %s\nPermissions : %s\nModifié : %s",
			info.Name,
			formatFileSize(info.Size),
			path,
			info.Permissions,
			info.ModTime.Format("02/01/2006 15:04:05"),
		)
	}

	dialog.ShowInformation("Propriétés", content, fo.window)
}

// formatFileSize formats a file size in human-readable form.
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
