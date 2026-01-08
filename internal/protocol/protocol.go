// Package protocol defines the common interface for file transfer protocols.
package protocol

import (
	"context"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// Performance constants
const (
	// DefaultBufferSize is the buffer size for file transfers (256KB for optimal performance)
	DefaultBufferSize = 256 * 1024

	// LargeFileThreshold is the size above which we use larger buffers (10MB)
	LargeFileThreshold = 10 * 1024 * 1024

	// LargeBufferSize is used for files larger than LargeFileThreshold (1MB)
	LargeBufferSize = 1024 * 1024
)

// GetOptimalBufferSize returns the optimal buffer size based on file size.
func GetOptimalBufferSize(fileSize int64) int {
	if fileSize > LargeFileThreshold {
		return LargeBufferSize
	}
	return DefaultBufferSize
}

// CopyWithBuffer copies from src to dst using an optimized buffer size.
func CopyWithBuffer(dst io.Writer, src io.Reader, fileSize int64) (int64, error) {
	bufSize := GetOptimalBufferSize(fileSize)
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}

// FileInfo represents information about a remote file or directory.
type FileInfo struct {
	Name        string
	Size        int64
	IsDir       bool
	ModTime     time.Time
	Permissions string
}

// TransferProgress represents the progress of a file transfer.
type TransferProgress struct {
	FileName       string
	TotalBytes     int64
	TransferredBytes int64
	BytesPerSecond int64
	StartTime      time.Time
}

// HostKeyCallback is a function called to verify SSH host keys.
type HostKeyCallback func(hostname string, remote net.Addr, key ssh.PublicKey) error

// ConnectionConfig holds the configuration for a connection.
type ConnectionConfig struct {
	Protocol   string // "sftp", "ftps", or "ftp"
	Host       string
	Port       int
	Username   string
	Password   string
	PrivateKey []byte // For SFTP key-based auth
	Timeout    time.Duration

	// TLS settings for FTPS
	TLSImplicit   bool // true for implicit FTPS (port 990)
	TLSSkipVerify bool // Skip certificate verification (not recommended)

	// SSH settings for SFTP
	HostKeyCallback HostKeyCallback // Callback for host key verification
}

// Protocol defines the interface that both SFTP and FTPS clients must implement.
type Protocol interface {
	// Connect establishes a connection to the remote server.
	Connect(ctx context.Context, config *ConnectionConfig) error

	// Disconnect closes the connection.
	Disconnect() error

	// IsConnected returns true if currently connected.
	IsConnected() bool

	// List returns the contents of a directory.
	List(ctx context.Context, path string) ([]FileInfo, error)

	// Stat returns information about a file or directory.
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// Mkdir creates a directory.
	Mkdir(ctx context.Context, path string) error

	// Remove removes a file.
	Remove(ctx context.Context, path string) error

	// RemoveDir removes a directory.
	RemoveDir(ctx context.Context, path string) error

	// Rename renames a file or directory.
	Rename(ctx context.Context, oldPath, newPath string) error

	// Upload uploads a file to the remote server.
	// progressFn is called periodically with transfer progress.
	Upload(ctx context.Context, localPath, remotePath string, resume bool, progressFn func(TransferProgress)) error

	// Download downloads a file from the remote server.
	// progressFn is called periodically with transfer progress.
	Download(ctx context.Context, remotePath, localPath string, resume bool, progressFn func(TransferProgress)) error

	// GetReader returns a reader for a remote file (for streaming).
	GetReader(ctx context.Context, path string) (io.ReadCloser, error)

	// GetWriter returns a writer for a remote file (for streaming).
	GetWriter(ctx context.Context, path string, append bool) (io.WriteCloser, error)

	// CurrentDir returns the current working directory.
	CurrentDir() (string, error)

	// ChangeDir changes the current working directory.
	ChangeDir(path string) error

	// GetProtocolName returns the protocol name ("sftp" or "ftps").
	GetProtocolName() string
}

// ProgressWriter wraps an io.Writer to track transfer progress.
type ProgressWriter struct {
	Writer     io.Writer
	TotalSize  int64
	Written    int64
	StartTime  time.Time
	FileName   string
	ProgressFn func(TransferProgress)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.Written += int64(n)

	if pw.ProgressFn != nil {
		elapsed := time.Since(pw.StartTime).Seconds()
		var speed int64
		if elapsed > 0 {
			speed = int64(float64(pw.Written) / elapsed)
		}

		pw.ProgressFn(TransferProgress{
			FileName:         pw.FileName,
			TotalBytes:       pw.TotalSize,
			TransferredBytes: pw.Written,
			BytesPerSecond:   speed,
			StartTime:        pw.StartTime,
		})
	}

	return n, err
}

// ProgressReader wraps an io.Reader to track transfer progress.
type ProgressReader struct {
	Reader     io.Reader
	TotalSize  int64
	BytesRead  int64
	StartTime  time.Time
	FileName   string
	ProgressFn func(TransferProgress)
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.BytesRead += int64(n)

	if pr.ProgressFn != nil {
		elapsed := time.Since(pr.StartTime).Seconds()
		var speed int64
		if elapsed > 0 {
			speed = int64(float64(pr.BytesRead) / elapsed)
		}

		pr.ProgressFn(TransferProgress{
			FileName:         pr.FileName,
			TotalBytes:       pr.TotalSize,
			TransferredBytes: pr.BytesRead,
			BytesPerSecond:   speed,
			StartTime:        pr.StartTime,
		})
	}

	return n, err
}
