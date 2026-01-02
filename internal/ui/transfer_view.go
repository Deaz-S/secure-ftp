// Package ui provides the transfer progress view component.
package ui

import (
	"fmt"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/transfer"
)

// TransferView displays the progress of file transfers.
type TransferView struct {
	container *fyne.Container
	list      *widget.List
	items     []*transfer.TransferItem
	mu        sync.RWMutex
}

// NewTransferView creates a new transfer view.
func NewTransferView() *TransferView {
	tv := &TransferView{
		items: make([]*transfer.TransferItem, 0),
	}

	tv.buildUI()
	return tv
}

// buildUI constructs the transfer view UI.
func (tv *TransferView) buildUI() {
	// Header
	header := container.NewHBox(
		widget.NewLabelWithStyle("Transfers", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	// Transfer list
	tv.list = widget.NewList(
		func() int {
			tv.mu.RLock()
			defer tv.mu.RUnlock()
			return len(tv.items)
		},
		func() fyne.CanvasObject {
			return container.NewVBox(
				container.NewHBox(
					widget.NewIcon(theme.UploadIcon()),
					widget.NewLabel("filename.txt"),
					widget.NewLabel("→"),
					widget.NewLabel("/remote/path"),
					widget.NewLabel("- 50%"),
					widget.NewLabel("1.2 MB/s"),
				),
				widget.NewProgressBar(),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			tv.mu.RLock()
			defer tv.mu.RUnlock()

			if id >= len(tv.items) {
				return
			}

			item := tv.items[id]
			box := obj.(*fyne.Container)

			// Info row
			infoRow := box.Objects[0].(*fyne.Container)

			// Direction icon
			icon := infoRow.Objects[0].(*widget.Icon)
			if item.Direction == transfer.DirectionUpload {
				icon.SetResource(theme.UploadIcon())
			} else {
				icon.SetResource(theme.DownloadIcon())
			}

			// Filename
			nameLabel := infoRow.Objects[1].(*widget.Label)
			nameLabel.SetText(filepath.Base(item.LocalPath))

			// Remote path
			remoteLabel := infoRow.Objects[3].(*widget.Label)
			remoteLabel.SetText(item.RemotePath)

			// Progress percentage
			progressLabel := infoRow.Objects[4].(*widget.Label)
			progressLabel.SetText(fmt.Sprintf("%.1f%%", item.Progress()))

			// Speed
			speedLabel := infoRow.Objects[5].(*widget.Label)
			speedLabel.SetText(formatSpeed(item.BytesPerSecond))

			// Progress bar
			progressBar := box.Objects[1].(*widget.ProgressBar)
			progressBar.SetValue(item.Progress() / 100)

			// Color based on status
			switch item.Status {
			case transfer.StatusCompleted:
				progressLabel.SetText("Complete")
			case transfer.StatusFailed:
				progressLabel.SetText("Failed")
			case transfer.StatusCancelled:
				progressLabel.SetText("Cancelled")
			case transfer.StatusPaused:
				progressLabel.SetText("Paused")
			}
		},
	)

	// Clear button
	clearBtn := widget.NewButtonWithIcon("Clear Completed", theme.DeleteIcon(), tv.clearCompleted)

	// Footer
	footer := container.NewHBox(
		clearBtn,
	)

	tv.container = container.NewBorder(
		header,
		footer,
		nil, nil,
		tv.list,
	)
}

// GetContainer returns the transfer view's container.
func (tv *TransferView) GetContainer() *fyne.Container {
	return tv.container
}

// AddTransfer adds a transfer to the view.
func (tv *TransferView) AddTransfer(item *transfer.TransferItem) {
	tv.mu.Lock()
	tv.items = append(tv.items, item)
	tv.mu.Unlock()
	tv.list.Refresh()
}

// UpdateTransfer updates a transfer in the view.
func (tv *TransferView) UpdateTransfer(item *transfer.TransferItem) {
	tv.mu.Lock()
	// Find and update the item
	for i, existing := range tv.items {
		if existing.ID == item.ID {
			tv.items[i] = item
			break
		}
	}
	tv.mu.Unlock()
	tv.list.Refresh()
}

// RemoveTransfer removes a transfer from the view.
func (tv *TransferView) RemoveTransfer(id string) {
	tv.mu.Lock()
	for i, item := range tv.items {
		if item.ID == id {
			tv.items = append(tv.items[:i], tv.items[i+1:]...)
			break
		}
	}
	tv.mu.Unlock()
	tv.list.Refresh()
}

// clearCompleted removes completed transfers from the view.
func (tv *TransferView) clearCompleted() {
	tv.mu.Lock()
	var remaining []*transfer.TransferItem
	for _, item := range tv.items {
		if item.Status != transfer.StatusCompleted &&
			item.Status != transfer.StatusFailed &&
			item.Status != transfer.StatusCancelled {
			remaining = append(remaining, item)
		}
	}
	tv.items = remaining
	tv.mu.Unlock()
	tv.list.Refresh()
}

// GetActiveCount returns the number of active transfers.
func (tv *TransferView) GetActiveCount() int {
	tv.mu.RLock()
	defer tv.mu.RUnlock()

	count := 0
	for _, item := range tv.items {
		if item.Status == transfer.StatusInProgress || item.Status == transfer.StatusPending {
			count++
		}
	}
	return count
}

// formatSpeed formats transfer speed in human-readable form.
func formatSpeed(bytesPerSecond int64) string {
	if bytesPerSecond == 0 {
		return "0 B/s"
	}

	const unit = 1024
	if bytesPerSecond < unit {
		return fmt.Sprintf("%d B/s", bytesPerSecond)
	}
	div, exp := int64(unit), 0
	for n := bytesPerSecond / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", float64(bytesPerSecond)/float64(div), "KMGTPE"[exp])
}
