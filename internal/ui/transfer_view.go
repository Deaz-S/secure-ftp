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

	// Callbacks for transfer actions
	onPause  func(id string)
	onResume func(id string)
	onCancel func(id string)
	onRetry  func(id string)
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
		widget.NewLabelWithStyle("Transferts", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	// Transfer list
	tv.list = widget.NewList(
		func() int {
			tv.mu.RLock()
			defer tv.mu.RUnlock()
			return len(tv.items)
		},
		func() fyne.CanvasObject {
			// Create action buttons (hidden by default, shown based on status)
			pauseBtn := widget.NewButtonWithIcon("", theme.MediaPauseIcon(), nil)
			pauseBtn.Importance = widget.LowImportance
			resumeBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), nil)
			resumeBtn.Importance = widget.LowImportance
			cancelBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
			cancelBtn.Importance = widget.LowImportance
			retryBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), nil)
			retryBtn.Importance = widget.LowImportance

			return container.NewVBox(
				container.NewHBox(
					widget.NewIcon(theme.UploadIcon()),
					widget.NewLabel("filename.txt"),
					widget.NewLabel("→"),
					widget.NewLabel("/remote/path"),
				),
				container.NewHBox(
					widget.NewProgressBar(),
					widget.NewLabel("- 50%"),
					widget.NewLabel("1.2 MB/s"),
					pauseBtn,
					resumeBtn,
					cancelBtn,
					retryBtn,
				),
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

			// Info row (first row)
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

			// Progress row (second row)
			progressRow := box.Objects[1].(*fyne.Container)

			// Progress bar
			progressBar := progressRow.Objects[0].(*widget.ProgressBar)
			progressBar.SetValue(item.Progress() / 100)

			// Progress percentage
			progressLabel := progressRow.Objects[1].(*widget.Label)
			progressLabel.SetText(fmt.Sprintf("%.1f%%", item.Progress()))

			// Speed
			speedLabel := progressRow.Objects[2].(*widget.Label)
			speedLabel.SetText(formatSpeed(item.BytesPerSecond))

			// Action buttons
			pauseBtn := progressRow.Objects[3].(*widget.Button)
			resumeBtn := progressRow.Objects[4].(*widget.Button)
			cancelBtn := progressRow.Objects[5].(*widget.Button)
			retryBtn := progressRow.Objects[6].(*widget.Button)

			// Copy item ID for closure
			itemID := item.ID

			// Set button callbacks
			pauseBtn.OnTapped = func() {
				if tv.onPause != nil {
					tv.onPause(itemID)
				}
			}
			resumeBtn.OnTapped = func() {
				if tv.onResume != nil {
					tv.onResume(itemID)
				}
			}
			cancelBtn.OnTapped = func() {
				if tv.onCancel != nil {
					tv.onCancel(itemID)
				}
			}
			retryBtn.OnTapped = func() {
				if tv.onRetry != nil {
					tv.onRetry(itemID)
				}
			}

			// Show/hide buttons based on status
			pauseBtn.Hide()
			resumeBtn.Hide()
			cancelBtn.Hide()
			retryBtn.Hide()

			switch item.Status {
			case transfer.StatusPending:
				pauseBtn.Show()
				cancelBtn.Show()
				progressLabel.SetText("En attente")
				speedLabel.SetText("")
			case transfer.StatusInProgress:
				cancelBtn.Show()
				// Note: pause during transfer is complex, would need protocol support
			case transfer.StatusPaused:
				resumeBtn.Show()
				cancelBtn.Show()
				progressLabel.SetText("En pause")
				speedLabel.SetText("")
			case transfer.StatusCompleted:
				progressLabel.SetText("Terminé")
				speedLabel.SetText("")
			case transfer.StatusFailed:
				retryBtn.Show()
				progressLabel.SetText("Échec")
				speedLabel.SetText("")
			case transfer.StatusCancelled:
				progressLabel.SetText("Annulé")
				speedLabel.SetText("")
			}
		},
	)

	// Clear button
	clearBtn := widget.NewButtonWithIcon("Effacer terminés", theme.DeleteIcon(), tv.clearCompleted)

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

// SetOnPause sets the callback for pause action.
func (tv *TransferView) SetOnPause(fn func(id string)) {
	tv.onPause = fn
}

// SetOnResume sets the callback for resume action.
func (tv *TransferView) SetOnResume(fn func(id string)) {
	tv.onResume = fn
}

// SetOnCancel sets the callback for cancel action.
func (tv *TransferView) SetOnCancel(fn func(id string)) {
	tv.onCancel = fn
}

// SetOnRetry sets the callback for retry action.
func (tv *TransferView) SetOnRetry(fn func(id string)) {
	tv.onRetry = fn
}
