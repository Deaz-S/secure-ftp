// Package ui provides the graphical user interface using Fyne.
package ui

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"

	"secure-ftp/internal/config"
	"secure-ftp/internal/protocol"
	ftpsync "secure-ftp/internal/sync"
	"secure-ftp/internal/transfer"
	"secure-ftp/pkg/logger"
)

// MainWindow represents the main application window.
type MainWindow struct {
	app           fyne.App
	window        fyne.Window
	configMgr     *config.ConfigManager
	credentialsMgr *config.CredentialsManager
	log           *logger.Logger

	// Connection state
	client         protocol.Protocol
	transferMgr    *transfer.TransferManager
	connected      bool
	currentProfile *config.ConnectionProfile

	// Security
	knownHosts *config.KnownHostsManager

	// Drag & Drop
	dragDropMgr *DragDropManager

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

	// Apply theme from settings
	mw.applyTheme(cfg.Theme)

	mw.app.SetIcon(AppIcon)
	mw.window = mw.app.NewWindow("Secure FTP - Client de transfert sécurisé")
	mw.window.SetIcon(AppIcon)
	mw.window.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))

	// Initialize known hosts manager for SFTP security
	// Use ~/.config/secure-ftp as config directory (not the logs subdirectory)
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "secure-ftp")
	knownHosts, err := config.NewKnownHostsManager(configDir)
	if err != nil {
		mw.log.Warnf("Failed to initialize known hosts manager: %v", err)
	} else {
		mw.knownHosts = knownHosts
	}

	// Initialize credentials manager with a default master password
	// In a production app, this should prompt the user for a master password
	credsMgr, err := config.NewCredentialsManager(configDir, "secure-ftp-master")
	if err != nil {
		mw.log.Warnf("Failed to initialize credentials manager: %v", err)
	} else {
		mw.credentialsMgr = credsMgr
	}

	mw.buildUI()

	return mw
}

// buildUI constructs the user interface.
func (mw *MainWindow) buildUI() {
	// Create toolbar
	toolbar := mw.createToolbar()

	// Create file browsers
	cfg := mw.configMgr.Get()
	mw.localBrowser = NewFileBrowser(mw.window, true, cfg.DefaultLocalDir)
	mw.localBrowser.SetShowHidden(cfg.ShowHiddenFiles)
	mw.remoteBrowser = NewFileBrowser(mw.window, false, "/")
	mw.remoteBrowser.SetShowHidden(cfg.ShowHiddenFiles)
	mw.remoteBrowser.SetDisabled(true) // Disabled until connected

	// Initialize drag & drop manager
	mw.dragDropMgr = NewDragDropManager(mw.window)
	mw.dragDropMgr.SetBrowsers(mw.localBrowser, mw.remoteBrowser)
	mw.dragDropMgr.SetOnUpload(func(localPath string) {
		if mw.connected {
			mw.uploadFile(localPath)
		}
	})
	mw.dragDropMgr.SetOnDownload(func(remotePath string) {
		if mw.connected {
			mw.downloadFile(remotePath)
		}
	})

	// Set up drag start callbacks for browsers
	mw.localBrowser.SetOnDragStart(func(items []FileItem) {
		mw.dragDropMgr.StartDrag(mw.localBrowser, items)
	})
	mw.remoteBrowser.SetOnDragStart(func(items []FileItem) {
		if mw.connected {
			mw.dragDropMgr.StartDrag(mw.remoteBrowser, items)
		}
	})

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
	mw.statusBar = widget.NewLabel("Déconnecté")

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
	mw.connectBtn = widget.NewButtonWithIcon("Connexion", theme.ComputerIcon(), mw.onConnect)
	mw.disconnectBtn = widget.NewButtonWithIcon("Déconnexion", theme.CancelIcon(), mw.onDisconnect)
	mw.disconnectBtn.Disable()

	refreshBtn := widget.NewButtonWithIcon("Actualiser", theme.ViewRefreshIcon(), mw.onRefresh)
	uploadBtn := widget.NewButtonWithIcon("Envoyer", theme.UploadIcon(), mw.onUpload)
	downloadBtn := widget.NewButtonWithIcon("Télécharger", theme.DownloadIcon(), mw.onDownload)
	syncBtn := widget.NewButtonWithIcon("Synchroniser", theme.MediaReplayIcon(), mw.onSync)

	// Drag indicator label (shown during drag operations)
	dragLabel := widget.NewLabel("")
	dragLabel.Hide()

	return container.NewHBox(
		mw.connectBtn,
		mw.disconnectBtn,
		widget.NewSeparator(),
		refreshBtn,
		layout.NewSpacer(),
		dragLabel,
		uploadBtn,
		downloadBtn,
		syncBtn,
	)
}

// createMenu creates the application menu.
func (mw *MainWindow) createMenu() {
	fileMenu := fyne.NewMenu("Fichier",
		fyne.NewMenuItem("Connexion...", mw.onConnect),
		fyne.NewMenuItem("Déconnexion", mw.onDisconnect),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Profils...", mw.onManageProfiles),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quitter", func() { mw.app.Quit() }),
	)

	editMenu := fyne.NewMenu("Édition",
		fyne.NewMenuItem("Paramètres...", mw.onSettings),
	)

	transferMenu := fyne.NewMenu("Transfert",
		fyne.NewMenuItem("Envoyer la sélection", mw.onUpload),
		fyne.NewMenuItem("Télécharger la sélection", mw.onDownload),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Synchroniser les dossiers...", mw.onSync),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Annuler tout", mw.onCancelAll),
	)

	helpMenu := fyne.NewMenu("Aide",
		fyne.NewMenuItem("À propos", mw.onAbout),
	)

	mainMenu := fyne.NewMainMenu(fileMenu, editMenu, transferMenu, helpMenu)
	mw.window.SetMainMenu(mainMenu)
}

// setupCallbacks sets up event handlers.
func (mw *MainWindow) setupCallbacks() {
	// Create file operations handler
	fileOps := NewFileOperations(mw.window)

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

	// Local file operations
	mw.localBrowser.SetOnNewFolder(func() {
		fileOps.CreateFolderLocal(mw.localBrowser.GetCurrentPath(), func() {
			mw.localBrowser.Refresh()
		})
	})

	mw.localBrowser.SetOnDelete(func() {
		item := mw.localBrowser.GetSelectedItem()
		if item != nil {
			fileOps.DeleteLocal(item.Path, item.IsDir, func() {
				mw.localBrowser.Refresh()
			})
		}
	})

	mw.localBrowser.SetOnRename(func() {
		item := mw.localBrowser.GetSelectedItem()
		if item != nil {
			fileOps.RenameLocal(item.Path, func() {
				mw.localBrowser.Refresh()
			})
		}
	})

	// Remote file operations
	mw.remoteBrowser.SetOnNewFolder(func() {
		if mw.connected {
			fileOps.SetClient(mw.client)
			fileOps.CreateFolderRemote(mw.remoteBrowser.GetCurrentPath(), func() {
				mw.remoteBrowser.Refresh()
			})
		}
	})

	mw.remoteBrowser.SetOnDelete(func() {
		if mw.connected {
			item := mw.remoteBrowser.GetSelectedItem()
			if item != nil {
				fileOps.SetClient(mw.client)
				fileOps.DeleteRemote(item.Path, item.IsDir, func() {
					mw.remoteBrowser.Refresh()
				})
			}
		}
	})

	mw.remoteBrowser.SetOnRename(func() {
		if mw.connected {
			item := mw.remoteBrowser.GetSelectedItem()
			if item != nil {
				fileOps.SetClient(mw.client)
				fileOps.RenameRemote(item.Path, func() {
					mw.remoteBrowser.Refresh()
				})
			}
		}
	})

	// Transfer view callbacks
	mw.transferView.SetOnPause(func(id string) {
		if mw.transferMgr != nil {
			mw.transferMgr.Pause(id)
		}
	})

	mw.transferView.SetOnResume(func(id string) {
		if mw.transferMgr != nil {
			mw.transferMgr.Resume(id)
		}
	})

	mw.transferView.SetOnCancel(func(id string) {
		if mw.transferMgr != nil {
			mw.transferMgr.Cancel(id)
		}
	})

	mw.transferView.SetOnRetry(func(id string) {
		if mw.transferMgr != nil {
			if newItem, err := mw.transferMgr.Retry(id); err == nil {
				mw.transferView.AddTransfer(newItem)
			}
		}
	})
}

// onConnect handles the connect button click.
func (mw *MainWindow) onConnect() {
	dlg := NewConnectionDialog(mw.window, mw.configMgr, mw.credentialsMgr, func(profile *config.ConnectionProfile, password string) {
		mw.connect(profile, password)
	})
	dlg.Show()
}

// connect establishes a connection to the server.
func (mw *MainWindow) connect(profile *config.ConnectionProfile, password string) {
	mw.statusBar.SetText(fmt.Sprintf("Connexion à %s...", profile.Host))

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

		// Set up host key verification for SFTP
		if profile.Protocol == "sftp" {
			if mw.knownHosts != nil {
				connConfig.HostKeyCallback = mw.createHostKeyCallback()
			} else {
				// Fallback: accept all host keys (less secure but allows connection)
				connConfig.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
					mw.log.Warnf("Host key verification disabled - accepting key for %s", hostname)
					return nil
				}
			}
		}

		// Load private key if specified
		if profile.PrivateKeyPath != "" {
			keyData, err := os.ReadFile(profile.PrivateKeyPath)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Échec de lecture de la clé privée : %w", err), mw.window)
				mw.statusBar.SetText("Échec de connexion")
				return
			}
			connConfig.PrivateKey = keyData
		}

		ctx := context.Background()
		if err := client.Connect(ctx, connConfig); err != nil {
			mw.window.Canvas().Refresh(mw.statusBar)
			dialog.ShowError(fmt.Errorf("Échec de connexion : %w", err), mw.window)
			mw.statusBar.SetText("Échec de connexion")
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
		mw.statusBar.SetText(fmt.Sprintf("Connecté à %s", profile.Host))

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
	mw.statusBar.SetText("Déconnecté")
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
		dialog.ShowInformation("Non connecté", "Veuillez d'abord vous connecter à un serveur.", mw.window)
		return
	}

	selected := mw.localBrowser.GetSelectedFiles()
	if len(selected) == 0 {
		dialog.ShowInformation("Aucune sélection", "Veuillez sélectionner des fichiers à envoyer.", mw.window)
		return
	}

	remoteDir := mw.remoteBrowser.GetCurrentPath()
	for _, localPath := range selected {
		mw.uploadFile(localPath)
	}

	mw.statusBar.SetText(fmt.Sprintf("Envoi de %d fichier(s) vers %s", len(selected), remoteDir))
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
		dialog.ShowInformation("Non connecté", "Veuillez d'abord vous connecter à un serveur.", mw.window)
		return
	}

	selected := mw.remoteBrowser.GetSelectedFiles()
	if len(selected) == 0 {
		dialog.ShowInformation("Aucune sélection", "Veuillez sélectionner des fichiers à télécharger.", mw.window)
		return
	}

	for _, remotePath := range selected {
		mw.downloadFile(remotePath)
	}

	mw.statusBar.SetText(fmt.Sprintf("Téléchargement de %d fichier(s)", len(selected)))
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
		dialog.ShowInformation("Non connecté", "Veuillez d'abord vous connecter à un serveur.", mw.window)
		return
	}

	localDir := mw.localBrowser.GetCurrentPath()
	remoteDir := mw.remoteBrowser.GetCurrentPath()

	dlg := NewSyncDialog(mw.window, localDir, remoteDir, mw.performSync)
	dlg.Show()
}

// performSync executes folder synchronization.
func (mw *MainWindow) performSync(options ftpsync.SyncOptions, localDir, remoteDir string) {
	mw.statusBar.SetText("Synchronisation des dossiers...")

	go func() {
		syncer := ftpsync.NewSyncer(mw.client, mw.transferMgr, options)
		result, err := syncer.Execute(context.Background(), localDir, remoteDir)

		if err != nil {
			dialog.ShowError(err, mw.window)
			mw.statusBar.SetText("Échec de synchronisation")
			return
		}

		// Show results
		msg := fmt.Sprintf("Synchronisation terminée !\n\n"+
			"Envoyés : %d fichiers\n"+
			"Téléchargés : %d fichiers\n"+
			"Supprimés : %d fichiers\n"+
			"Ignorés : %d fichiers\n"+
			"Total transféré : %s\n"+
			"Durée : %s",
			result.FilesUploaded,
			result.FilesDownloaded,
			result.FilesDeleted,
			result.FilesSkipped,
			formatBytes(result.BytesTransferred),
			result.Duration.Round(time.Millisecond),
		)

		if len(result.Errors) > 0 {
			msg += fmt.Sprintf("\n\nErreurs : %d", len(result.Errors))
		}

		if options.DryRun {
			msg = "[SIMULATION - Aucune modification effectuée]\n\n" + msg
		}

		dialog.ShowInformation("Synchronisation terminée", msg, mw.window)
		mw.statusBar.SetText("Synchronisation terminée")

		// Refresh both browsers
		mw.localBrowser.Refresh()
		mw.remoteBrowser.Refresh()
	}()
}

// onCancelAll cancels all transfers.
func (mw *MainWindow) onCancelAll() {
	if mw.transferMgr != nil {
		mw.transferMgr.CancelAll()
		mw.statusBar.SetText("Tous les transferts annulés")
	}
}

// onManageProfiles opens the profiles management dialog.
func (mw *MainWindow) onManageProfiles() {
	dlg := NewProfilesDialog(mw.window, mw.configMgr, mw.credentialsMgr, func() {
		// Callback when profiles are updated
	})
	dlg.Show()
}

// onSettings opens the settings dialog.
func (mw *MainWindow) onSettings() {
	dlg := NewSettingsDialog(mw.window, mw.configMgr, func() {
		// Apply all settings changes
		mw.applySettings()
	})
	dlg.Show()
}

// onAbout shows the about dialog.
func (mw *MainWindow) onAbout() {
	dialog.ShowInformation("À propos de Secure FTP",
		"Secure FTP Client\nVersion 1.0.0\n\nClient de transfert de fichiers sécurisé supportant les protocoles SFTP et FTPS.",
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
		mw.statusBar.SetText(fmt.Sprintf("Transfert terminé : %s", item.LocalPath))
	} else if item.Status == transfer.StatusFailed {
		mw.statusBar.SetText(fmt.Sprintf("Échec du transfert : %s", item.Error))
	}
}

// createHostKeyCallback creates a callback for SSH host key verification.
func (mw *MainWindow) createHostKeyCallback() protocol.HostKeyCallback {
	callback := mw.knownHosts.GetHostKeyCallback()
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return callback(hostname, remote, key)
	}
}

// setupKnownHostsCallbacks sets up the callbacks for host key verification dialogs.
func (mw *MainWindow) setupKnownHostsCallbacks() {
	if mw.knownHosts == nil {
		return
	}

	// Callback for new hosts
	onNewHost := func(host string, fingerprint string) bool {
		var accepted bool
		var wg sync.WaitGroup
		wg.Add(1)

		// Show dialog on main thread
		mw.window.Canvas().Refresh(mw.statusBar)
		dialog.ShowConfirm(
			"Nouvel hôte SSH",
			fmt.Sprintf("L'authenticité de l'hôte '%s' ne peut pas être vérifiée.\n\n"+
				"Empreinte : %s\n\n"+
				"Voulez-vous faire confiance à cet hôte et continuer la connexion ?", host, fingerprint),
			func(confirm bool) {
				accepted = confirm
				wg.Done()
			},
			mw.window,
		)

		wg.Wait()
		return accepted
	}

	// Callback for changed hosts (security warning)
	onChanged := func(host string, oldFP, newFP string) bool {
		var accepted bool
		var wg sync.WaitGroup
		wg.Add(1)

		// Show warning dialog on main thread
		mw.window.Canvas().Refresh(mw.statusBar)
		dialog.ShowConfirm(
			"ATTENTION : CLÉ HÔTE MODIFIÉE !",
			fmt.Sprintf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
				"@  ATTENTION : POSSIBLE ATTAQUE MAN-IN-THE-MIDDLE ! @\n"+
				"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n\n"+
				"La clé de l'hôte '%s' a changé !\n\n"+
				"Ancienne empreinte : %s\n"+
				"Nouvelle empreinte : %s\n\n"+
				"Cela peut signifier :\n"+
				"- Le serveur a été réinstallé\n"+
				"- Quelqu'un intercepte la connexion (attaque MITM)\n\n"+
				"N'acceptez que si vous êtes CERTAIN que c'est normal !", host, oldFP, newFP),
			func(confirm bool) {
				accepted = confirm
				wg.Done()
			},
			mw.window,
		)

		wg.Wait()
		return accepted
	}

	mw.knownHosts.SetCallbacks(onNewHost, onChanged)
}

// Run starts the application.
func (mw *MainWindow) Run() {
	// Set up host key verification callbacks
	mw.setupKnownHostsCallbacks()

	// Set up external file drop handler
	mw.window.SetOnDropped(mw.handleExternalDrop)

	// Show connection dialog on startup
	mw.window.SetOnClosed(func() {})
	go func() {
		// Small delay to ensure window is ready
		mw.onConnect()
	}()
	mw.window.ShowAndRun()
}

// handleExternalDrop handles files dropped from the OS file manager.
func (mw *MainWindow) handleExternalDrop(pos fyne.Position, uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}

	// Count files to upload
	fileCount := 0
	for _, uri := range uris {
		// Check if it's a file (not directory)
		info, err := os.Stat(uri.Path())
		if err == nil && !info.IsDir() {
			fileCount++
		}
	}

	if fileCount == 0 {
		dialog.ShowInformation("Glisser-déposer", "Aucun fichier valide détecté.", mw.window)
		return
	}

	if mw.connected {
		// Upload dropped files to remote server
		remoteDir := mw.remoteBrowser.GetCurrentPath()
		for _, uri := range uris {
			info, err := os.Stat(uri.Path())
			if err == nil && !info.IsDir() {
				mw.uploadFile(uri.Path())
			}
		}
		mw.statusBar.SetText(fmt.Sprintf("%d fichier(s) déposé(s) - envoi vers %s", fileCount, remoteDir))
	} else {
		// Not connected - show message
		dialog.ShowInformation("Non connecté",
			fmt.Sprintf("%d fichier(s) déposé(s).\nConnectez-vous à un serveur pour envoyer les fichiers.", fileCount),
			mw.window)
	}
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

// formatBytes formats bytes into a human-readable string.
func formatBytes(bytes int64) string {
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

// applyTheme applies the theme setting.
func (mw *MainWindow) applyTheme(themeName string) {
	switch themeName {
	case "dark":
		mw.app.Settings().SetTheme(theme.DarkTheme())
	case "light":
		mw.app.Settings().SetTheme(theme.LightTheme())
	default:
		// "system" - use default
	}
}

// applySettings applies all settings from config.
func (mw *MainWindow) applySettings() {
	cfg := mw.configMgr.Get()

	// Apply theme
	mw.applyTheme(cfg.Theme)

	// Apply window size
	mw.window.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))

	// Apply show hidden files
	mw.localBrowser.SetShowHidden(cfg.ShowHiddenFiles)
	mw.remoteBrowser.SetShowHidden(cfg.ShowHiddenFiles)

	// Apply transfer settings
	if mw.transferMgr != nil {
		mw.transferMgr.SetMaxParallel(cfg.MaxParallelTransfers)
	}
}
