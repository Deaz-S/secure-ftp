// Package config provides known hosts management for SFTP security.
package config

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// HostKeyStatus represents the status of a host key verification.
type HostKeyStatus int

const (
	// HostKeyNew indicates a new host key not in known_hosts.
	HostKeyNew HostKeyStatus = iota
	// HostKeyValid indicates the host key matches known_hosts.
	HostKeyValid
	// HostKeyChanged indicates the host key has changed (possible attack).
	HostKeyChanged
)

// KnownHostsManager manages SSH known hosts.
type KnownHostsManager struct {
	filePath   string
	hosts      map[string]string // host:port -> fingerprint
	mu         sync.RWMutex
	onNewHost  func(host string, fingerprint string) bool // Returns true to accept
	onChanged  func(host string, oldFP, newFP string) bool // Returns true to accept (dangerous)
}

// NewKnownHostsManager creates a new known hosts manager.
func NewKnownHostsManager(configDir string) (*KnownHostsManager, error) {
	filePath := filepath.Join(configDir, "known_hosts")

	mgr := &KnownHostsManager{
		filePath: filePath,
		hosts:    make(map[string]string),
	}

	// Load existing known hosts
	if err := mgr.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load known_hosts: %w", err)
	}

	return mgr, nil
}

// SetCallbacks sets the callback functions for host key verification.
func (m *KnownHostsManager) SetCallbacks(onNewHost func(string, string) bool, onChanged func(string, string, string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNewHost = onNewHost
	m.onChanged = onChanged
}

// load reads the known_hosts file.
func (m *KnownHostsManager) load() error {
	file, err := os.Open(m.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			m.hosts[parts[0]] = parts[1]
		}
	}

	return scanner.Err()
}

// save writes the known_hosts file.
func (m *KnownHostsManager) save() error {
	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	file, err := os.OpenFile(m.filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	for host, fingerprint := range m.hosts {
		fmt.Fprintf(file, "%s %s\n", host, fingerprint)
	}

	return nil
}

// GetFingerprint computes the SHA256 fingerprint of a public key.
func GetFingerprint(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyHostKey verifies a host's public key.
func (m *KnownHostsManager) VerifyHostKey(host string, port int, key ssh.PublicKey) (HostKeyStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hostKey := fmt.Sprintf("[%s]:%d", host, port)
	fingerprint := GetFingerprint(key)

	storedFP, exists := m.hosts[hostKey]

	if !exists {
		// New host
		return HostKeyNew, nil
	}

	if storedFP == fingerprint {
		// Valid, key matches
		return HostKeyValid, nil
	}

	// Key has changed - possible MITM attack!
	return HostKeyChanged, nil
}

// AddHost adds a new host to known_hosts.
func (m *KnownHostsManager) AddHost(host string, port int, key ssh.PublicKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hostKey := fmt.Sprintf("[%s]:%d", host, port)
	fingerprint := GetFingerprint(key)

	m.hosts[hostKey] = fingerprint

	return m.save()
}

// UpdateHost updates an existing host's key (use with caution).
func (m *KnownHostsManager) UpdateHost(host string, port int, key ssh.PublicKey) error {
	return m.AddHost(host, port, key)
}

// RemoveHost removes a host from known_hosts.
func (m *KnownHostsManager) RemoveHost(host string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hostKey := fmt.Sprintf("[%s]:%d", host, port)
	delete(m.hosts, hostKey)

	return m.save()
}

// GetHostKeyCallback returns an ssh.HostKeyCallback for use with ssh.ClientConfig.
func (m *KnownHostsManager) GetHostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Parse host and port from remote address
		host, portStr, err := net.SplitHostPort(remote.String())
		if err != nil {
			host = hostname
			portStr = "22"
		}

		port := 22
		fmt.Sscanf(portStr, "%d", &port)

		// Use hostname if host is IP
		if hostname != "" && host != hostname {
			host = hostname
		}

		status, err := m.VerifyHostKey(host, port, key)
		if err != nil {
			return err
		}

		fingerprint := GetFingerprint(key)

		switch status {
		case HostKeyValid:
			return nil

		case HostKeyNew:
			m.mu.RLock()
			callback := m.onNewHost
			m.mu.RUnlock()

			if callback != nil {
				if callback(host, fingerprint) {
					// User accepted, add to known hosts
					return m.AddHost(host, port, key)
				}
				return fmt.Errorf("host key rejected by user for %s", host)
			}
			// No callback, reject by default for security
			return fmt.Errorf("unknown host %s with fingerprint %s", host, fingerprint)

		case HostKeyChanged:
			m.mu.RLock()
			callback := m.onChanged
			storedFP := m.hosts[fmt.Sprintf("[%s]:%d", host, port)]
			m.mu.RUnlock()

			if callback != nil {
				if callback(host, storedFP, fingerprint) {
					// User accepted the risk, update host
					return m.UpdateHost(host, port, key)
				}
			}
			return fmt.Errorf("WARNING: HOST KEY HAS CHANGED for %s! Possible man-in-the-middle attack", host)
		}

		return fmt.Errorf("unknown host key status")
	}
}
