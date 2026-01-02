// Package ui provides the graphical user interface using Fyne.
package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/config"
	"secure-ftp/internal/protocol"
	"secure-ftp/internal/transfer"
	"secure-ftp/pkg/logger"
)

// MainWindow represents the main application window.
type MainWindow struct {
	app           fyne.App
	window        fyne.Window
	configMgr     *config.ConfigManager
	log           *logger.Logger

	// Connection state
	client         protocol.Protocol
	transferMgr    *transfer.TransferManager
	connected      bool
	currentProfile *config.ConnectionProfile

	// UI components
	localBrowser    *FileBrowser
	remoteBrowser   *FileBrowser
	transferView    *TransferView
	statusBar       *widget.Label
	connectBtn      *widget.Button
	disconnectBtn   *widget.Button
}

// NewMainWindow creates and initializes the main application window.
func NewMainWindow(configMgr *config.ConfigManager) *MainWindow {
	mw := &MainWindow{
		app:       app.New(),
		configMgr: configMgr,
		log:       logger.GetInstance(),
	}

	cfg := configMgr.Get()
	mw.window = mw.app.NewWindow("Secure FTP")
	mw.window.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))

	mw.buildUI()

	return mw
}

// buildUI constructs the user interface.
func (mw *MainWindow) buildUI() {
	// Create toolbar
	toolbar := mw.createToolbar()

	// Create file browsers
	mw.localBrowser = NewFileBrowser(mw.window, true, mw.configMgr.Get().DefaultLocalDir)
	mw.remoteBrowser = NewFileBrowser(mw.window, false, "/")
	mw.remoteBrowser.SetDisabled(true) // Disabled until connected

	// Create transfer view
	mw.transferView = NewTransferView()

	// Create split view for browsers
	browserSplit := container.NewHSplit(
		mw.localBrowser.GetContainer(),
		mw.remoteBrowser.GetContainer(),
	)
	browserSplit.SetOffset(0.5)

	// Create main split with transfer view
	mainSplit := container.NewVSplit(
		browserSplit,
		mw.transferView.GetContainer(),
	)
	mainSplit.SetOffset(0.7)

	// Create status bar
	mw.statusBar = widget.NewLabel("Disconnected")

	// Main layout
	content := container.NewBorder(
		toolbar,           // top
		mw.statusBar,      // bottom
		nil,               // left
		nil,               // right
		mainSplit,         // center
	)

	mw.window.SetContent(content)

	// Set up callbacks
	mw.setupCallbacks()

	// Create main menu
	mw.createMenu()
}

// createToolbar creates the main toolbar.
func (mw *MainWindow) createToolbar() *fyne.Container {
	mw.connectBtn = widget.NewButtonWithIcon("Connect", theme.ComputerIcon(), mw.onConnect)
	mw.disconnectBtn = widget.NewButtonWithIcon("Disconnect", theme.CancelIcon(), mw.onDisconnect)
	mw.disconnectBtn.Disable()

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), mw.onRefresh)
	uploadBtn := widget.NewButtonWithIcon("Upload", theme.UploadIcon(), mw.onUpload)
	downloadBtn := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), mw.onDownload)
	syncBtn := widget.NewButtonWithIcon("Sync", theme.MediaReplayIcon(), mw.onSync)

	return container.NewHBox(
		mw.connectBtn,
		mw.disconnectBtn,
		widget.NewSeparator(),
		refreshBtn,
		layout.NewSpacer(),
		uploadBtn,
		downloadBtn,
		syncBtn,
	)
}

// createMenu creates the application menu.
func (mw *MainWindow) createMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Connect...", mw.onConnect),
		fyne.NewMenuItem("Disconnect", mw.onDisconnect),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Profiles...", mw.onManageProfiles),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() { mw.app.Quit() }),
	)

	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Settings...", mw.onSettings),
	)

	transferMenu := fyne.NewMenu("Transfer",
		fyne.NewMenuItem("Upload Selected", mw.onUpload),
		fyne.NewMenuItem("Download Selected", mw.onDownload),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Sync Folders...", mw.onSync),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Cancel All", mw.onCancelAll),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("About", mw.onAbout),
	)

	mainMenu := fyne.NewMainMenu(fileMenu, editMenu, transferMenu, helpMenu)
	mw.window.SetMainMenu(mainMenu)
}

// setupCallbacks sets up event handlers.
func (mw *MainWindow) setupCallbacks() {
	// Double-click on local file to upload
	mw.localBrowser.SetOnFileDoubleClick(func(path string, isDir bool) {
		if !isDir && mw.connected {
			mw.uploadFile(path)
		}
	})

	// Double-click on remote file to download
	mw.remoteBrowser.SetOnFileDoubleClick(func(path string, isDir bool) {
		if !isDir && mw.connected {
			mw.downloadFile(path)
		}
	})
}

// onConnect handles the connect button click.
func (mw *MainWindow) onConnect() {
	dlg := NewConnectionDialog(mw.window, mw.configMgr, func(profile *config.ConnectionProfile, password string) {
		mw.connect(profile, password)
	})
	dlg.Show()
}

// connect establishes a connection to the server.
func (mw *MainWindow) connect(profile *config.ConnectionProfile, password string) {
	mw.statusBar.SetText(fmt.Sprintf("Connecting to %s...", profile.Host))

	go func() {
		// Create appropriate client
		var client protocol.Protocol
		if profile.Protocol == "sftp" {
			client = protocol.NewSFTPClient()
		} else {
			// FTP and FTPS both use FTPSClient
			client = protocol.NewFTPSClient()
		}

		// Build connection config
		connConfig := &protocol.ConnectionConfig{
			Protocol:      profile.Protocol,
			Host:          profile.Host,
			Port:          profile.Port,
			Username:      profile.Username,
			Password:      password,
			TLSImplicit:   profile.TLSImplicit,
		}

		// Load private key if specified
		if profile.PrivateKeyPath != "" {
			// TODO: Load private key from file
		}

		ctx := context.Background()
		if err := client.Connect(ctx, connConfig); err != nil {
			mw.window.Canvas().Refresh(mw.statusBar)
			dialog.ShowError(fmt.Errorf("Connection failed: %w", err), mw.window)
			mw.statusBar.SetText("Connection failed")
			return
		}

		// Success
		mw.client = client
		mw.connected = true
		mw.currentProfile = profile

		// Update last used
		mw.configMgr.UpdateLastUsed(profile.ID)

		// Create transfer manager
		cfg := mw.configMgr.Get()
		mw.transferMgr = transfer.NewTransferManager(client, cfg.MaxParallelTransfers)
		mw.transferMgr.SetUpdateCallback(mw.onTransferUpdate)
		mw.transferMgr.SetCompleteCallback(mw.onTransferComplete)

		// Update UI
		mw.updateConnectionState()
		mw.statusBar.SetText(fmt.Sprintf("Connected to %s", profile.Host))

		// Refresh remote browser
		mw.remoteBrowser.SetClient(client)
		startDir := "/"
		if profile.RemoteDir != "" {
			startDir = profile.RemoteDir
		}
		mw.remoteBrowser.NavigateTo(startDir)
	}()
}

// onDisconnect handles the disconnect button click.
func (mw *MainWindow) onDisconnect() {
	if mw.client != nil {
		mw.client.Disconnect()
	}

	mw.client = nil
	mw.connected = false
	mw.currentProfile = nil

	if mw.transferMgr != nil {
		mw.transferMgr.Stop()
		mw.transferMgr = nil
	}

	mw.updateConnectionState()
	mw.statusBar.SetText("Disconnected")
}

// updateConnectionState updates UI based on connection state.
func (mw *MainWindow) updateConnectionState() {
	if mw.connected {
		mw.connectBtn.Disable()
		mw.disconnectBtn.Enable()
		mw.remoteBrowser.SetDisabled(false)
	} else {
		mw.connectBtn.Enable()
		mw.disconnectBtn.Disable()
		mw.remoteBrowser.SetDisabled(true)
		mw.remoteBrowser.Clear()
	}
}

// onRefresh refreshes both file browsers.
func (mw *MainWindow) onRefresh() {
	mw.localBrowser.Refresh()
	if mw.connected {
		mw.remoteBrowser.Refresh()
	}
}

// onUpload uploads selected local files.
func (mw *MainWindow) onUpload() {
	if !mw.connected {
		dialog.ShowInformation("Not Connected", "Please connect to a server first.", mw.window)
		return
	}

	selected := mw.localBrowser.GetSelectedFiles()
	if len(selected) == 0 {
		dialog.ShowInformation("No Selection", "Please select files to upload.", mw.window)
		return
	}

	remoteDir := mw.remoteBrowser.GetCurrentPath()
	for _, localPath := range selected {
		mw.uploadFile(localPath)
	}

	mw.statusBar.SetText(fmt.Sprintf("Uploading %d file(s) to %s", len(selected), remoteDir))
}

// uploadFile uploads a single file.
func (mw *MainWindow) uploadFile(localPath string) {
	if mw.transferMgr == nil {
		return
	}

	remoteDir := mw.remoteBrowser.GetCurrentPath()
	remotePath := remoteDir + "/" + mw.localBrowser.GetFileName(localPath)

	item := mw.transferMgr.AddUpload(localPath, remotePath, 0)
	mw.transferView.AddTransfer(item)
}

// onDownload downloads selected remote files.
func (mw *MainWindow) onDownload() {
	if !mw.connected {
		dialog.ShowInformation("Not Connected", "Please connect to a server first.", mw.window)
		return
	}

	selected := mw.remoteBrowser.GetSelectedFiles()
	if len(selected) == 0 {
		dialog.ShowInformation("No Selection", "Please select files to download.", mw.window)
		return
	}

	for _, remotePath := range selected {
		mw.downloadFile(remotePath)
	}

	mw.statusBar.SetText(fmt.Sprintf("Downloading %d file(s)", len(selected)))
}

// downloadFile downloads a single file.
func (mw *MainWindow) downloadFile(remotePath string) {
	if mw.transferMgr == nil {
		return
	}

	localDir := mw.localBrowser.GetCurrentPath()
	localPath := localDir + "/" + mw.remoteBrowser.GetFileName(remotePath)

	item := mw.transferMgr.AddDownload(remotePath, localPath, 0)
	mw.transferView.AddTransfer(item)
}

// onSync opens the sync dialog.
func (mw *MainWindow) onSync() {
	if !mw.connected {
		dialog.ShowInformation("Not Connected", "Please connect to a server first.", mw.window)
		return
	}

	localDir := mw.localBrowser.GetCurrentPath()
	remoteDir := mw.remoteBrowser.GetCurrentPath()

	dialog.ShowConfirm("Sync Folders",
		fmt.Sprintf("Sync local folder:\n%s\n\nWith remote folder:\n%s", localDir, remoteDir),
		func(confirmed bool) {
			if confirmed {
				mw.performSync(localDir, remoteDir)
			}
		},
		mw.window,
	)
}

// performSync executes folder synchronization.
func (mw *MainWindow) performSync(localDir, remoteDir string) {
	mw.statusBar.SetText("Synchronizing folders...")

	// TODO: Implement sync dialog with options
	dialog.ShowInformation("Sync", "Folder synchronization will be implemented.", mw.window)
}

// onCancelAll cancels all transfers.
func (mw *MainWindow) onCancelAll() {
	if mw.transferMgr != nil {
		mw.transferMgr.CancelAll()
		mw.statusBar.SetText("All transfers cancelled")
	}
}

// onManageProfiles opens the profiles management dialog.
func (mw *MainWindow) onManageProfiles() {
	// TODO: Implement profiles dialog
	dialog.ShowInformation("Profiles", "Profile management will be implemented.", mw.window)
}

// onSettings opens the settings dialog.
func (mw *MainWindow) onSettings() {
	// TODO: Implement settings dialog
	dialog.ShowInformation("Settings", "Settings dialog will be implemented.", mw.window)
}

// onAbout shows the about dialog.
func (mw *MainWindow) onAbout() {
	dialog.ShowInformation("About Secure FTP",
		"Secure FTP Client\nVersion 1.0.0\n\nA secure file transfer client supporting SFTP and FTPS protocols.",
		mw.window)
}

// onTransferUpdate handles transfer progress updates.
func (mw *MainWindow) onTransferUpdate(item *transfer.TransferItem) {
	mw.transferView.UpdateTransfer(item)
}

// onTransferComplete handles transfer completion.
func (mw *MainWindow) onTransferComplete(item *transfer.TransferItem) {
	mw.transferView.UpdateTransfer(item)

	// Refresh the appropriate browser
	if item.Direction == transfer.DirectionUpload {
		mw.remoteBrowser.Refresh()
	} else {
		mw.localBrowser.Refresh()
	}

	if item.Status == transfer.StatusCompleted {
		mw.statusBar.SetText(fmt.Sprintf("Transfer complete: %s", item.LocalPath))
	} else if item.Status == transfer.StatusFailed {
		mw.statusBar.SetText(fmt.Sprintf("Transfer failed: %s", item.Error))
	}
}

// Run starts the application.
func (mw *MainWindow) Run() {
	// Show connection dialog on startup
	mw.window.SetOnClosed(func() {})
	go func() {
		// Small delay to ensure window is ready
		mw.onConnect()
	}()
	mw.window.ShowAndRun()
}

// Cleanup performs cleanup before exit.
func (mw *MainWindow) Cleanup() {
	if mw.client != nil {
		mw.client.Disconnect()
	}
	if mw.transferMgr != nil {
		mw.transferMgr.Stop()
	}
}
