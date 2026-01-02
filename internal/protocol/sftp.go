// Package protocol provides SFTP client implementation.
package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPClient implements the Protocol interface for SFTP connections.
type SFTPClient struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	connected  bool
	currentDir string
}

// NewSFTPClient creates a new SFTP client instance.
func NewSFTPClient() *SFTPClient {
	return &SFTPClient{}
}

// Connect establishes an SFTP connection to the remote server.
func (c *SFTPClient) Connect(ctx context.Context, config *ConnectionConfig) error {
	if c.connected {
		return fmt.Errorf("already connected")
	}

	// Build SSH auth methods
	var authMethods []ssh.AuthMethod

	// Password authentication
	if config.Password != "" {
		authMethods = append(authMethods, ssh.Password(config.Password))
	}

	// Private key authentication
	if len(config.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(config.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication method provided")
	}

	// Set default timeout
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User:            config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		Timeout:         timeout,
	}

	// Connect to SSH server
	address := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// Use context for connection timeout
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", address, err)
	}

	// Establish SSH connection
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, sshConfig)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SSH handshake failed: %w", err)
	}

	c.sshClient = ssh.NewClient(sshConn, chans, reqs)

	// Create SFTP client
	c.sftpClient, err = sftp.NewClient(c.sshClient)
	if err != nil {
		c.sshClient.Close()
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}

	c.connected = true
	c.currentDir, _ = c.sftpClient.Getwd()

	return nil
}

// Disconnect closes the SFTP and SSH connections.
func (c *SFTPClient) Disconnect() error {
	if !c.connected {
		return nil
	}

	var errs []error

	if c.sftpClient != nil {
		if err := c.sftpClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("SFTP close: %w", err))
		}
	}

	if c.sshClient != nil {
		if err := c.sshClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("SSH close: %w", err))
		}
	}

	c.connected = false
	c.sftpClient = nil
	c.sshClient = nil

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// IsConnected returns true if the client is connected.
func (c *SFTPClient) IsConnected() bool {
	return c.connected
}

// List returns the contents of a directory.
func (c *SFTPClient) List(ctx context.Context, path string) ([]FileInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	entries, err := c.sftpClient.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		files = append(files, FileInfo{
			Name:        entry.Name(),
			Size:        entry.Size(),
			IsDir:       entry.IsDir(),
			ModTime:     entry.ModTime(),
			Permissions: entry.Mode().String(),
		})
	}

	return files, nil
}

// Stat returns information about a file or directory.
func (c *SFTPClient) Stat(ctx context.Context, path string) (*FileInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	info, err := c.sftpClient.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	return &FileInfo{
		Name:        info.Name(),
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		ModTime:     info.ModTime(),
		Permissions: info.Mode().String(),
	}, nil
}

// Mkdir creates a directory.
func (c *SFTPClient) Mkdir(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.sftpClient.Mkdir(path)
}

// Remove removes a file.
func (c *SFTPClient) Remove(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.sftpClient.Remove(path)
}

// RemoveDir removes a directory.
func (c *SFTPClient) RemoveDir(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.sftpClient.RemoveDirectory(path)
}

// Rename renames a file or directory.
func (c *SFTPClient) Rename(ctx context.Context, oldPath, newPath string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.sftpClient.Rename(oldPath, newPath)
}

// Upload uploads a file to the remote server with optional resume support.
func (c *SFTPClient) Upload(ctx context.Context, localPath, remotePath string, resume bool, progressFn func(TransferProgress)) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get local file size
	localInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}
	totalSize := localInfo.Size()

	var remoteFile *sftp.File
	var startOffset int64

	if resume {
		// Check if remote file exists and get its size
		remoteInfo, err := c.sftpClient.Stat(remotePath)
		if err == nil && !remoteInfo.IsDir() {
			startOffset = remoteInfo.Size()
			if startOffset >= totalSize {
				// File already fully uploaded
				return nil
			}

			// Seek local file to resume position
			if _, err := localFile.Seek(startOffset, io.SeekStart); err != nil {
				return fmt.Errorf("failed to seek local file: %w", err)
			}

			// Open remote file for append
			remoteFile, err = c.sftpClient.OpenFile(remotePath, os.O_WRONLY|os.O_APPEND)
			if err != nil {
				return fmt.Errorf("failed to open remote file for append: %w", err)
			}
		}
	}

	if remoteFile == nil {
		// Create new remote file
		remoteFile, err = c.sftpClient.Create(remotePath)
		if err != nil {
			return fmt.Errorf("failed to create remote file: %w", err)
		}
		startOffset = 0
	}
	defer remoteFile.Close()

	// Create progress wrapper
	reader := &ProgressReader{
		Reader:     localFile,
		TotalSize:  totalSize,
		BytesRead:  startOffset,
		StartTime:  time.Now(),
		FileName:   filepath.Base(localPath),
		ProgressFn: progressFn,
	}

	// Copy with context cancellation support
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(remoteFile, reader)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
	}

	return nil
}

// Download downloads a file from the remote server with optional resume support.
func (c *SFTPClient) Download(ctx context.Context, remotePath, localPath string, resume bool, progressFn func(TransferProgress)) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Get remote file info
	remoteInfo, err := c.sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote file: %w", err)
	}
	totalSize := remoteInfo.Size()

	// Open remote file
	remoteFile, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	var localFile *os.File
	var startOffset int64

	if resume {
		// Check if local file exists
		localInfo, err := os.Stat(localPath)
		if err == nil && !localInfo.IsDir() {
			startOffset = localInfo.Size()
			if startOffset >= totalSize {
				// File already fully downloaded
				return nil
			}

			// Seek remote file to resume position
			if _, err := remoteFile.Seek(startOffset, io.SeekStart); err != nil {
				return fmt.Errorf("failed to seek remote file: %w", err)
			}

			// Open local file for append
			localFile, err = os.OpenFile(localPath, os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open local file for append: %w", err)
			}
		}
	}

	if localFile == nil {
		// Ensure directory exists
		dir := filepath.Dir(localPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Create new local file
		localFile, err = os.Create(localPath)
		if err != nil {
			return fmt.Errorf("failed to create local file: %w", err)
		}
		startOffset = 0
	}
	defer localFile.Close()

	// Create progress wrapper
	writer := &ProgressWriter{
		Writer:     localFile,
		TotalSize:  totalSize,
		Written:    startOffset,
		StartTime:  time.Now(),
		FileName:   filepath.Base(remotePath),
		ProgressFn: progressFn,
	}

	// Copy with context cancellation support
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(writer, remoteFile)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	return nil
}

// GetReader returns a reader for a remote file.
func (c *SFTPClient) GetReader(ctx context.Context, path string) (io.ReadCloser, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	return c.sftpClient.Open(path)
}

// GetWriter returns a writer for a remote file.
func (c *SFTPClient) GetWriter(ctx context.Context, path string, append bool) (io.WriteCloser, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	flags := os.O_WRONLY | os.O_CREATE
	if append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	return c.sftpClient.OpenFile(path, flags)
}

// CurrentDir returns the current working directory.
func (c *SFTPClient) CurrentDir() (string, error) {
	if !c.connected {
		return "", fmt.Errorf("not connected")
	}

	return c.sftpClient.Getwd()
}

// ChangeDir changes the current working directory.
func (c *SFTPClient) ChangeDir(path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Verify the directory exists
	info, err := c.sftpClient.Stat(path)
	if err != nil {
		return fmt.Errorf("directory does not exist: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}

	c.currentDir = path
	return nil
}

// GetProtocolName returns "sftp".
func (c *SFTPClient) GetProtocolName() string {
	return "sftp"
}
