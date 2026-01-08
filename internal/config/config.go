// Package config handles application configuration and connection profiles.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// ConnectionProfile stores connection settings for a server.
type ConnectionProfile struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Protocol   string        `json:"protocol"` // "sftp" or "ftps"
	Host       string        `json:"host"`
	Port       int           `json:"port"`
	Username   string        `json:"username"`
	// Note: Password is NOT stored for security
	PrivateKeyPath string    `json:"private_key_path,omitempty"`
	RemoteDir      string    `json:"remote_dir,omitempty"`
	LocalDir       string    `json:"local_dir,omitempty"`
	TLSImplicit    bool      `json:"tls_implicit,omitempty"`
	Timeout        int       `json:"timeout_seconds,omitempty"`
	LastUsed       time.Time `json:"last_used,omitempty"`
}

// AppConfig holds the application configuration.
type AppConfig struct {
	Profiles             []ConnectionProfile `json:"profiles"`
	MaxParallelTransfers int                 `json:"max_parallel_transfers"`
	LogLevel             string              `json:"log_level"`
	LogPath              string              `json:"log_path"`
	Theme                string              `json:"theme"` // "light", "dark", "system"
	WindowWidth          int                 `json:"window_width"`
	WindowHeight         int                 `json:"window_height"`
	ShowHiddenFiles      bool                `json:"show_hidden_files"`
	DefaultLocalDir      string              `json:"default_local_dir"`
	ResumeStatePath      string              `json:"resume_state_path"`
	// Bandwidth limits (bytes per second, 0 = unlimited)
	UploadRateLimit      int64               `json:"upload_rate_limit"`
	DownloadRateLimit    int64               `json:"download_rate_limit"`
	// Desktop notifications
	EnableNotifications  bool                `json:"enable_notifications"`
}

// ConfigManager handles loading and saving configuration.
type ConfigManager struct {
	config   *AppConfig
	path     string
	mu       sync.RWMutex
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *AppConfig {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "secure-ftp")

	return &AppConfig{
		Profiles:             make([]ConnectionProfile, 0),
		MaxParallelTransfers: 4,
		LogLevel:             "info",
		LogPath:              filepath.Join(configDir, "logs", "secure-ftp.log"),
		Theme:                "system",
		WindowWidth:          1200,
		WindowHeight:         800,
		ShowHiddenFiles:      false,
		DefaultLocalDir:      homeDir,
		ResumeStatePath:      filepath.Join(configDir, "resume.json"),
		UploadRateLimit:      0, // Unlimited by default
		DownloadRateLimit:    0, // Unlimited by default
		EnableNotifications:  true,
	}
}

// NewConfigManager creates a new config manager.
func NewConfigManager(configPath string) (*ConfigManager, error) {
	cm := &ConfigManager{
		path: configPath,
	}

	if err := cm.Load(); err != nil {
		// Use default config if file doesn't exist
		if os.IsNotExist(err) {
			cm.config = DefaultConfig()
			return cm, nil
		}
		return nil, err
	}

	return cm, nil
}

// Load reads the configuration from disk.
func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return err
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return err
	}

	cm.config = config
	return nil
}

// Save writes the configuration to disk.
func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(cm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cm.path, data, 0600)
}

// Get returns a copy of the current configuration.
func (cm *ConfigManager) Get() AppConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return *cm.config
}

// Set updates the configuration.
func (cm *ConfigManager) Set(config *AppConfig) error {
	cm.mu.Lock()
	cm.config = config
	cm.mu.Unlock()
	return cm.Save()
}

// AddProfile adds a new connection profile.
func (cm *ConfigManager) AddProfile(profile ConnectionProfile) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Generate ID if not set
	if profile.ID == "" {
		profile.ID = generateProfileID()
	}

	cm.config.Profiles = append(cm.config.Profiles, profile)
	return cm.save()
}

// UpdateProfile updates an existing profile.
func (cm *ConfigManager) UpdateProfile(profile ConnectionProfile) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, p := range cm.config.Profiles {
		if p.ID == profile.ID {
			cm.config.Profiles[i] = profile
			return cm.save()
		}
	}

	return nil
}

// DeleteProfile removes a profile by ID.
func (cm *ConfigManager) DeleteProfile(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, p := range cm.config.Profiles {
		if p.ID == id {
			cm.config.Profiles = append(cm.config.Profiles[:i], cm.config.Profiles[i+1:]...)
			return cm.save()
		}
	}

	return nil
}

// GetProfile returns a profile by ID.
func (cm *ConfigManager) GetProfile(id string) *ConnectionProfile {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, p := range cm.config.Profiles {
		if p.ID == id {
			return &p
		}
	}

	return nil
}

// GetProfiles returns all profiles.
func (cm *ConfigManager) GetProfiles() []ConnectionProfile {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]ConnectionProfile, len(cm.config.Profiles))
	copy(result, cm.config.Profiles)
	return result
}

// UpdateLastUsed updates the last used timestamp for a profile.
func (cm *ConfigManager) UpdateLastUsed(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, p := range cm.config.Profiles {
		if p.ID == id {
			cm.config.Profiles[i].LastUsed = time.Now()
			return cm.save()
		}
	}

	return nil
}

// save writes config without locking (caller must hold lock).
func (cm *ConfigManager) save() error {
	dir := filepath.Dir(cm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cm.path, data, 0600)
}

var profileCounter int

func generateProfileID() string {
	profileCounter++
	return "profile-" + time.Now().Format("20060102150405") + "-" + strconv.Itoa(profileCounter)
}
