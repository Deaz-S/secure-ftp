// Package ui provides context menu support for file operations.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ContextMenuItem represents a single menu item.
type ContextMenuItem struct {
	Label    string
	Icon     fyne.Resource
	Action   func()
	Disabled bool
}

// ContextMenu provides a popup context menu.
type ContextMenu struct {
	menu *fyne.Menu
}

// NewContextMenu creates a new context menu from items.
func NewContextMenu(items []ContextMenuItem) *ContextMenu {
	menuItems := make([]*fyne.MenuItem, 0, len(items))

	for _, item := range items {
		if item.Label == "-" {
			menuItems = append(menuItems, fyne.NewMenuItemSeparator())
		} else {
			mi := fyne.NewMenuItem(item.Label, item.Action)
			if item.Icon != nil {
				mi.Icon = item.Icon
			}
			mi.Disabled = item.Disabled
			menuItems = append(menuItems, mi)
		}
	}

	return &ContextMenu{
		menu: fyne.NewMenu("", menuItems...),
	}
}

// ShowAtPosition displays the context menu at the given canvas position.
func (cm *ContextMenu) ShowAtPosition(canvas fyne.Canvas, pos fyne.Position) {
	widget.ShowPopUpMenuAtPosition(cm.menu, canvas, pos)
}

// FileContextMenuItems returns standard context menu items for file operations.
func FileContextMenuItems(
	isDir bool,
	isRemote bool,
	onOpen func(),
	onDownloadUpload func(),
	onRename func(),
	onDelete func(),
	onNewFolder func(),
	onCopyPath func(),
	onRefresh func(),
) []ContextMenuItem {
	items := make([]ContextMenuItem, 0)

	// Open (for directories)
	if isDir {
		items = append(items, ContextMenuItem{
			Label:  "Ouvrir",
			Action: onOpen,
		})
	}

	// Download/Upload
	transferLabel := "Télécharger"
	if !isRemote {
		transferLabel = "Envoyer"
	}
	if onDownloadUpload != nil {
		items = append(items, ContextMenuItem{
			Label:    transferLabel,
			Action:   onDownloadUpload,
			Disabled: isDir,
		})
	}

	items = append(items, ContextMenuItem{Label: "-"})

	// Rename
	if onRename != nil {
		items = append(items, ContextMenuItem{
			Label:  "Renommer...",
			Action: onRename,
		})
	}

	// Delete
	if onDelete != nil {
		items = append(items, ContextMenuItem{
			Label:  "Supprimer",
			Action: onDelete,
		})
	}

	items = append(items, ContextMenuItem{Label: "-"})

	// New folder
	if onNewFolder != nil {
		items = append(items, ContextMenuItem{
			Label:  "Nouveau dossier...",
			Action: onNewFolder,
		})
	}

	// Copy path
	if onCopyPath != nil {
		items = append(items, ContextMenuItem{
			Label:  "Copier le chemin",
			Action: onCopyPath,
		})
	}

	items = append(items, ContextMenuItem{Label: "-"})

	// Refresh
	if onRefresh != nil {
		items = append(items, ContextMenuItem{
			Label:  "Actualiser",
			Action: onRefresh,
		})
	}

	return items
}

// EmptyContextMenuItems returns context menu items when nothing is selected.
func EmptyContextMenuItems(
	onNewFolder func(),
	onRefresh func(),
	onPaste func(),
	hasPaste bool,
) []ContextMenuItem {
	items := make([]ContextMenuItem, 0)

	// Paste
	if onPaste != nil {
		items = append(items, ContextMenuItem{
			Label:    "Coller",
			Action:   onPaste,
			Disabled: !hasPaste,
		})
	}

	items = append(items, ContextMenuItem{Label: "-"})

	// New folder
	if onNewFolder != nil {
		items = append(items, ContextMenuItem{
			Label:  "Nouveau dossier...",
			Action: onNewFolder,
		})
	}

	// Refresh
	if onRefresh != nil {
		items = append(items, ContextMenuItem{
			Label:  "Actualiser",
			Action: onRefresh,
		})
	}

	return items
}
