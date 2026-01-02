// Package protocol provides FTPS client implementation.
package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jlaffaye/ftp"
)

// FTPSClient implements the Protocol interface for FTPS connections.
type FTPSClient struct {
	conn       *ftp.ServerConn
	connected  bool
	currentDir string
	config     *ConnectionConfig
}

// NewFTPSClient creates a new FTPS client instance.
func NewFTPSClient() *FTPSClient {
	return &FTPSClient{}
}

// Connect establishes an FTP/FTPS connection to the remote server.
func (c *FTPSClient) Connect(ctx context.Context, config *ConnectionConfig) error {
	if c.connected {
		return fmt.Errorf("already connected")
	}

	c.config = config

	// Set default timeout
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	address := fmt.Sprintf("%s:%d", config.Host, config.Port)

	var conn *ftp.ServerConn
	var err error

	if config.Protocol == "ftp" {
		// Plain FTP (no TLS)
		conn, err = ftp.Dial(address,
			ftp.DialWithTimeout(timeout),
			ftp.DialWithContext(ctx),
		)
	} else {
		// TLS configuration for FTPS
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
			ServerName:         config.Host,
			MinVersion:         tls.VersionTLS12,
		}

		if config.TLSImplicit {
			// Implicit FTPS (port 990 typically)
			conn, err = ftp.Dial(address,
				ftp.DialWithTimeout(timeout),
				ftp.DialWithTLS(tlsConfig),
				ftp.DialWithContext(ctx),
			)
		} else {
			// Explicit FTPS (AUTH TLS)
			conn, err = ftp.Dial(address,
				ftp.DialWithTimeout(timeout),
				ftp.DialWithExplicitTLS(tlsConfig),
				ftp.DialWithContext(ctx),
			)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Login
	if err := conn.Login(config.Username, config.Password); err != nil {
		conn.Quit()
		return fmt.Errorf("login failed: %w", err)
	}

	c.conn = conn
	c.connected = true

	// Get current directory
	c.currentDir, _ = conn.CurrentDir()

	return nil
}

// Disconnect closes the FTPS connection.
func (c *FTPSClient) Disconnect() error {
	if !c.connected {
		return nil
	}

	err := c.conn.Quit()
	c.conn = nil
	c.connected = false

	return err
}

// IsConnected returns true if the client is connected.
func (c *FTPSClient) IsConnected() bool {
	return c.connected
}

// List returns the contents of a directory.
func (c *FTPSClient) List(ctx context.Context, path string) ([]FileInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	entries, err := c.conn.List(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		isDir := entry.Type == ftp.EntryTypeFolder

		// Convert FTP permissions
		perms := "-rw-r--r--"
		if isDir {
			perms = "drwxr-xr-x"
		}

		files = append(files, FileInfo{
			Name:        entry.Name,
			Size:        int64(entry.Size),
			IsDir:       isDir,
			ModTime:     entry.Time,
			Permissions: perms,
		})
	}

	return files, nil
}

// Stat returns information about a file or directory.
func (c *FTPSClient) Stat(ctx context.Context, path string) (*FileInfo, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	// FTP doesn't have a direct stat command, we need to list the parent directory
	dir := filepath.Dir(path)
	name := filepath.Base(path)

	entries, err := c.conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	for _, entry := range entries {
		if entry.Name == name {
			isDir := entry.Type == ftp.EntryTypeFolder
			perms := "-rw-r--r--"
			if isDir {
				perms = "drwxr-xr-x"
			}

			return &FileInfo{
				Name:        entry.Name,
				Size:        int64(entry.Size),
				IsDir:       isDir,
				ModTime:     entry.Time,
				Permissions: perms,
			}, nil
		}
	}

	return nil, fmt.Errorf("file not found: %s", path)
}

// Mkdir creates a directory.
func (c *FTPSClient) Mkdir(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.conn.MakeDir(path)
}

// Remove removes a file.
func (c *FTPSClient) Remove(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.conn.Delete(path)
}

// RemoveDir removes a directory.
func (c *FTPSClient) RemoveDir(ctx context.Context, path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.conn.RemoveDir(path)
}

// Rename renames a file or directory.
func (c *FTPSClient) Rename(ctx context.Context, oldPath, newPath string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	return c.conn.Rename(oldPath, newPath)
}

// Upload uploads a file to the remote server with optional resume support.
func (c *FTPSClient) Upload(ctx context.Context, localPath, remotePath string, resume bool, progressFn func(TransferProgress)) error {
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

	var startOffset int64

	if resume {
		// Check remote file size using SIZE command
		remoteSize, err := c.conn.FileSize(remotePath)
		if err == nil && remoteSize > 0 {
			if remoteSize >= totalSize {
				// File already fully uploaded
				return nil
			}
			startOffset = remoteSize

			// Seek local file to resume position
			if _, err := localFile.Seek(startOffset, io.SeekStart); err != nil {
				return fmt.Errorf("failed to seek local file: %w", err)
			}

			// Use REST command for resume
			if err := c.conn.StorFrom(remotePath, localFile, uint64(startOffset)); err != nil {
				return fmt.Errorf("failed to resume upload: %w", err)
			}
			return nil
		}
	}

	// Create progress wrapper
	reader := &ProgressReader{
		Reader:     localFile,
		TotalSize:  totalSize,
		BytesRead:  startOffset,
		StartTime:  time.Now(),
		FileName:   filepath.Base(localPath),
		ProgressFn: progressFn,
	}

	// Upload file
	if err := c.conn.Stor(remotePath, reader); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	return nil
}

// Download downloads a file from the remote server with optional resume support.
func (c *FTPSClient) Download(ctx context.Context, remotePath, localPath string, resume bool, progressFn func(TransferProgress)) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Get remote file size
	remoteSize, err := c.conn.FileSize(remotePath)
	if err != nil {
		return fmt.Errorf("failed to get remote file size: %w", err)
	}

	var localFile *os.File
	var startOffset int64

	if resume {
		// Check if local file exists
		localInfo, err := os.Stat(localPath)
		if err == nil && !localInfo.IsDir() {
			startOffset = localInfo.Size()
			if startOffset >= remoteSize {
				// File already fully downloaded
				return nil
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
		TotalSize:  remoteSize,
		Written:    startOffset,
		StartTime:  time.Now(),
		FileName:   filepath.Base(remotePath),
		ProgressFn: progressFn,
	}

	var resp *ftp.Response
	if startOffset > 0 {
		// Resume download using REST command
		resp, err = c.conn.RetrFrom(remotePath, uint64(startOffset))
	} else {
		resp, err = c.conn.Retr(remotePath)
	}

	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer resp.Close()

	// Copy data
	if _, err := io.Copy(writer, resp); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	return nil
}

// GetReader returns a reader for a remote file.
func (c *FTPSClient) GetReader(ctx context.Context, path string) (io.ReadCloser, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	return c.conn.Retr(path)
}

// GetWriter returns a writer for a remote file.
func (c *FTPSClient) GetWriter(ctx context.Context, path string, appendMode bool) (io.WriteCloser, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}

	// FTP doesn't provide a direct writer interface
	// We need to use a pipe
	pr, pw := io.Pipe()

	go func() {
		var err error
		if appendMode {
			// For append, we need to get current size and use StorFrom
			size, sizeErr := c.conn.FileSize(path)
			if sizeErr == nil && size > 0 {
				err = c.conn.StorFrom(path, pr, uint64(size))
			} else {
				err = c.conn.Stor(path, pr)
			}
		} else {
			err = c.conn.Stor(path, pr)
		}
		if err != nil {
			pr.CloseWithError(err)
		}
	}()

	return pw, nil
}

// CurrentDir returns the current working directory.
func (c *FTPSClient) CurrentDir() (string, error) {
	if !c.connected {
		return "", fmt.Errorf("not connected")
	}

	return c.conn.CurrentDir()
}

// ChangeDir changes the current working directory.
func (c *FTPSClient) ChangeDir(path string) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	if err := c.conn.ChangeDir(path); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	c.currentDir, _ = c.conn.CurrentDir()
	return nil
}

// GetProtocolName returns "ftps".
func (c *FTPSClient) GetProtocolName() string {
	return "ftps"
}
