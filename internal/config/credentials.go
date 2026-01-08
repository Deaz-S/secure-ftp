// Package config provides secure credential storage with AES-GCM encryption.
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

// CredentialsManager handles secure storage and retrieval of passwords.
type CredentialsManager struct {
	path       string
	masterKey  []byte
	credentials map[string]string // profileID -> encrypted password
	mu         sync.RWMutex
}

// credentialsFile represents the stored credentials file format.
type credentialsFile struct {
	Salt        string            `json:"salt"`
	Credentials map[string]string `json:"credentials"`
}

const (
	pbkdf2Iterations = 100000
	keyLength        = 32 // AES-256
	saltLength       = 32
)

// NewCredentialsManager creates a new credentials manager.
// The masterPassword is used to encrypt/decrypt stored passwords.
func NewCredentialsManager(configDir string, masterPassword string) (*CredentialsManager, error) {
	cm := &CredentialsManager{
		path:        filepath.Join(configDir, "credentials.enc"),
		credentials: make(map[string]string),
	}

	// Load existing credentials or create new file
	if err := cm.load(masterPassword); err != nil {
		if os.IsNotExist(err) {
			// Generate new salt and derive key
			salt := make([]byte, saltLength)
			if _, err := rand.Read(salt); err != nil {
				return nil, fmt.Errorf("failed to generate salt: %w", err)
			}
			cm.masterKey = pbkdf2.Key([]byte(masterPassword), salt, pbkdf2Iterations, keyLength, sha256.New)

			// Save empty credentials file with salt
			if err := cm.saveWithSalt(salt); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return cm, nil
}

// load reads and decrypts the credentials file.
func (cm *CredentialsManager) load(masterPassword string) error {
	data, err := os.ReadFile(cm.path)
	if err != nil {
		return err
	}

	var cf credentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("failed to parse credentials file: %w", err)
	}

	// Decode salt
	salt, err := base64.StdEncoding.DecodeString(cf.Salt)
	if err != nil {
		return fmt.Errorf("failed to decode salt: %w", err)
	}

	// Derive master key from password
	cm.masterKey = pbkdf2.Key([]byte(masterPassword), salt, pbkdf2Iterations, keyLength, sha256.New)

	// Store encrypted credentials (will be decrypted on demand)
	cm.credentials = cf.Credentials

	return nil
}

// saveWithSalt saves the credentials file with a specific salt.
// Note: This method does NOT acquire a lock - caller must handle locking if needed.
func (cm *CredentialsManager) saveWithSalt(salt []byte) error {
	cf := credentialsFile{
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Credentials: cm.credentials,
	}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cm.path), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Write with secure permissions (owner read/write only)
	return os.WriteFile(cm.path, data, 0600)
}

// save saves the credentials file (reloads salt from existing file).
// Note: This method does NOT acquire a lock - called from locked contexts (SetPassword, DeletePassword).
func (cm *CredentialsManager) save() error {
	// Read existing salt
	data, err := os.ReadFile(cm.path)
	if err != nil {
		return err
	}

	var cf credentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return err
	}

	salt, err := base64.StdEncoding.DecodeString(cf.Salt)
	if err != nil {
		return err
	}

	return cm.saveWithSalt(salt)
}

// encrypt encrypts plaintext using AES-GCM.
func (cm *CredentialsManager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(cm.masterKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts ciphertext using AES-GCM.
func (cm *CredentialsManager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(cm.masterKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// SetPassword stores an encrypted password for a profile.
func (cm *CredentialsManager) SetPassword(profileID, password string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	encrypted, err := cm.encrypt(password)
	if err != nil {
		return err
	}

	cm.credentials[profileID] = encrypted
	return cm.save()
}

// GetPassword retrieves and decrypts a password for a profile.
func (cm *CredentialsManager) GetPassword(profileID string) (string, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	encrypted, exists := cm.credentials[profileID]
	if !exists {
		return "", nil // No password stored
	}

	return cm.decrypt(encrypted)
}

// DeletePassword removes the stored password for a profile.
func (cm *CredentialsManager) DeletePassword(profileID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.credentials, profileID)
	return cm.save()
}

// HasPassword checks if a password is stored for a profile.
func (cm *CredentialsManager) HasPassword(profileID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	_, exists := cm.credentials[profileID]
	return exists
}

// ChangeMasterPassword re-encrypts all passwords with a new master password.
func (cm *CredentialsManager) ChangeMasterPassword(oldPassword, newPassword string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Decrypt all passwords with old key
	decrypted := make(map[string]string)
	for id, encrypted := range cm.credentials {
		plaintext, err := cm.decrypt(encrypted)
		if err != nil {
			return fmt.Errorf("failed to decrypt password for %s: %w", id, err)
		}
		decrypted[id] = plaintext
	}

	// Generate new salt and derive new key
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}
	cm.masterKey = pbkdf2.Key([]byte(newPassword), salt, pbkdf2Iterations, keyLength, sha256.New)

	// Re-encrypt all passwords with new key
	cm.credentials = make(map[string]string)
	for id, plaintext := range decrypted {
		encrypted, err := cm.encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("failed to encrypt password for %s: %w", id, err)
		}
		cm.credentials[id] = encrypted
	}

	return cm.saveWithSalt(salt)
}

// VerifyMasterPassword checks if the provided password is correct.
func VerifyMasterPassword(configDir, password string) bool {
	cm := &CredentialsManager{
		path:        filepath.Join(configDir, "credentials.enc"),
		credentials: make(map[string]string),
	}

	// Try to load with the provided password
	if err := cm.load(password); err != nil {
		return false
	}

	// Try to decrypt a credential to verify (if any exist)
	for _, encrypted := range cm.credentials {
		_, err := cm.decrypt(encrypted)
		return err == nil
	}

	// No credentials to verify, assume password is correct for new file
	return true
}

// CredentialsFileExists checks if the credentials file exists.
func CredentialsFileExists(configDir string) bool {
	path := filepath.Join(configDir, "credentials.enc")
	_, err := os.Stat(path)
	return err == nil
}
