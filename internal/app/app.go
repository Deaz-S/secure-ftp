// Package app provides the main application logic.
package app

import (
	"os"
	"path/filepath"

	"secure-ftp/internal/config"
	"secure-ftp/internal/ui"
	"secure-ftp/pkg/logger"
)

// App represents the main application.
type App struct {
	configMgr  *config.ConfigManager
	log        *logger.Logger
	mainWindow *ui.MainWindow
}

// New creates a new application instance.
func New() (*App, error) {
	// Determine config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	configPath := filepath.Join(homeDir, ".config", "secure-ftp", "config.json")

	// Initialize config manager
	configMgr, err := config.NewConfigManager(configPath)
	if err != nil {
		return nil, err
	}

	// Initialize logger
	log := logger.GetInstance()
	cfg := configMgr.Get()
	err = log.Initialize(logger.Config{
		LogPath: cfg.LogPath,
		Level:   cfg.LogLevel,
		Console: true,
	})
	if err != nil {
		// Log error but continue
		log.Warnf("Failed to initialize file logging: %v", err)
	}

	app := &App{
		configMgr: configMgr,
		log:       log,
	}

	return app, nil
}

// Run starts the application.
func (a *App) Run() {
	a.log.Info("Starting Secure FTP application")

	// Create main window
	a.mainWindow = ui.NewMainWindow(a.configMgr)

	// Run the application
	a.mainWindow.Run()

	// Cleanup
	a.cleanup()
}

// cleanup performs cleanup before exit.
func (a *App) cleanup() {
	a.log.Info("Shutting down Secure FTP application")

	if a.mainWindow != nil {
		a.mainWindow.Cleanup()
	}

	// Save config
	a.configMgr.Save()

	// Close logger
	a.log.Close()
}
