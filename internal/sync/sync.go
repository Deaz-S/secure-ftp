// Package sync provides folder synchronization functionality.
package sync

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"secure-ftp/internal/protocol"
	"secure-ftp/internal/transfer"
	"secure-ftp/pkg/logger"
)

// SyncMode determines how files are synchronized.
type SyncMode int

const (
	// ModeUpload syncs local to remote (upload new/modified files).
	ModeUpload SyncMode = iota
	// ModeDownload syncs remote to local (download new/modified files).
	ModeDownload
	// ModeMirror makes remote exactly match local (including deletions).
	ModeMirror
	// ModeBidirectional syncs changes in both directions (newest wins).
	ModeBidirectional
)

// CompareMethod determines how files are compared.
type CompareMethod int

const (
	CompareByModTime CompareMethod = iota
	CompareBySize
	CompareByHash
	CompareBySizeAndTime
)

// SyncOptions configures synchronization behavior.
type SyncOptions struct {
	Mode          SyncMode
	CompareMethod CompareMethod
	ExcludePatterns []string   // Glob patterns to exclude
	IncludePatterns []string   // Glob patterns to include (if set, only these are synced)
	DeleteExtra     bool       // Delete files on destination not present on source
	DryRun          bool       // Don't actually transfer, just report what would happen
	IgnoreHidden    bool       // Skip hidden files (starting with .)
}

// SyncResult contains the results of a synchronization.
type SyncResult struct {
	FilesUploaded    int
	FilesDownloaded  int
	FilesDeleted     int
	FilesSkipped     int
	BytesTransferred int64
	Errors           []error
	Duration         time.Duration
}

// SyncAction represents a planned sync action.
type SyncAction struct {
	Type       string // "upload", "download", "delete_local", "delete_remote", "skip"
	LocalPath  string
	RemotePath string
	Reason     string
}

// Syncer handles folder synchronization.
type Syncer struct {
	client   protocol.Protocol
	manager  *transfer.TransferManager
	log      *logger.Logger
	options  SyncOptions
}

// NewSyncer creates a new syncer instance.
func NewSyncer(client protocol.Protocol, manager *transfer.TransferManager, options SyncOptions) *Syncer {
	return &Syncer{
		client:  client,
		manager: manager,
		log:     logger.GetInstance(),
		options: options,
	}
}

// Analyze compares local and remote directories and returns planned actions.
func (s *Syncer) Analyze(ctx context.Context, localDir, remoteDir string) ([]SyncAction, error) {
	var actions []SyncAction

	// Get local files
	localFiles, err := s.scanLocalDir(localDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan local directory: %w", err)
	}

	// Get remote files
	remoteFiles, err := s.scanRemoteDir(ctx, remoteDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan remote directory: %w", err)
	}

	// Create maps for lookup
	localMap := make(map[string]os.FileInfo)
	for _, f := range localFiles {
		relPath, _ := filepath.Rel(localDir, f.path)
		localMap[relPath] = f.info
	}

	remoteMap := make(map[string]protocol.FileInfo)
	for _, f := range remoteFiles {
		relPath := strings.TrimPrefix(f.path, remoteDir)
		relPath = strings.TrimPrefix(relPath, "/")
		remoteMap[relPath] = f.info
	}

	switch s.options.Mode {
	case ModeUpload:
		actions = s.analyzeUpload(localDir, remoteDir, localMap, remoteMap)
	case ModeDownload:
		actions = s.analyzeDownload(localDir, remoteDir, localMap, remoteMap)
	case ModeMirror:
		actions = s.analyzeMirror(localDir, remoteDir, localMap, remoteMap)
	case ModeBidirectional:
		actions = s.analyzeBidirectional(localDir, remoteDir, localMap, remoteMap)
	}

	return actions, nil
}

// Execute performs the synchronization.
func (s *Syncer) Execute(ctx context.Context, localDir, remoteDir string) (*SyncResult, error) {
	startTime := time.Now()
	result := &SyncResult{}

	actions, err := s.Analyze(ctx, localDir, remoteDir)
	if err != nil {
		return nil, err
	}

	if s.options.DryRun {
		// Just count what would happen
		for _, action := range actions {
			switch action.Type {
			case "upload":
				result.FilesUploaded++
			case "download":
				result.FilesDownloaded++
			case "delete_local", "delete_remote":
				result.FilesDeleted++
			case "skip":
				result.FilesSkipped++
			}
		}
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Execute actions
	for _, action := range actions {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.Duration = time.Since(startTime)
			return result, nil
		default:
		}

		switch action.Type {
		case "upload":
			if err := s.client.Upload(ctx, action.LocalPath, action.RemotePath, false, nil); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("upload %s: %w", action.LocalPath, err))
			} else {
				result.FilesUploaded++
				if info, err := os.Stat(action.LocalPath); err == nil {
					result.BytesTransferred += info.Size()
				}
			}

		case "download":
			if err := s.client.Download(ctx, action.RemotePath, action.LocalPath, false, nil); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("download %s: %w", action.RemotePath, err))
			} else {
				result.FilesDownloaded++
				if info, _ := s.client.Stat(ctx, action.RemotePath); info != nil {
					result.BytesTransferred += info.Size
				}
			}

		case "delete_local":
			if err := os.Remove(action.LocalPath); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("delete local %s: %w", action.LocalPath, err))
			} else {
				result.FilesDeleted++
			}

		case "delete_remote":
			if err := s.client.Remove(ctx, action.RemotePath); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("delete remote %s: %w", action.RemotePath, err))
			} else {
				result.FilesDeleted++
			}

		case "skip":
			result.FilesSkipped++
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

type localFileInfo struct {
	path string
	info os.FileInfo
}

type remoteFileInfo struct {
	path string
	info protocol.FileInfo
}

func (s *Syncer) scanLocalDir(dir string) ([]localFileInfo, error) {
	var files []localFileInfo

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip hidden files if configured
		if s.options.IgnoreHidden && strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Check exclude patterns
		relPath, _ := filepath.Rel(dir, path)
		if s.isExcluded(relPath) {
			return nil
		}

		files = append(files, localFileInfo{path: path, info: info})
		return nil
	})

	return files, err
}

func (s *Syncer) scanRemoteDir(ctx context.Context, dir string) ([]remoteFileInfo, error) {
	var files []remoteFileInfo

	var scan func(path string) error
	scan = func(path string) error {
		entries, err := s.client.List(ctx, path)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			fullPath := filepath.Join(path, entry.Name)

			// Skip hidden files if configured
			if s.options.IgnoreHidden && strings.HasPrefix(entry.Name, ".") {
				continue
			}

			// Check exclude patterns
			relPath := strings.TrimPrefix(fullPath, dir)
			relPath = strings.TrimPrefix(relPath, "/")
			if s.isExcluded(relPath) {
				continue
			}

			if entry.IsDir {
				if err := scan(fullPath); err != nil {
					return err
				}
			} else {
				files = append(files, remoteFileInfo{path: fullPath, info: entry})
			}
		}

		return nil
	}

	err := scan(dir)
	return files, err
}

func (s *Syncer) isExcluded(path string) bool {
	// Check exclude patterns
	for _, pattern := range s.options.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}

	// If include patterns are set, only include matching files
	if len(s.options.IncludePatterns) > 0 {
		for _, pattern := range s.options.IncludePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return false
			}
			if matched, _ := filepath.Match(pattern, path); matched {
				return false
			}
		}
		return true
	}

	return false
}

func (s *Syncer) needsSync(localInfo os.FileInfo, remoteInfo protocol.FileInfo) bool {
	switch s.options.CompareMethod {
	case CompareBySize:
		return localInfo.Size() != remoteInfo.Size

	case CompareByModTime:
		// Allow 2 second tolerance for time comparison
		diff := localInfo.ModTime().Sub(remoteInfo.ModTime)
		if diff < 0 {
			diff = -diff
		}
		return diff > 2*time.Second

	case CompareBySizeAndTime:
		if localInfo.Size() != remoteInfo.Size {
			return true
		}
		diff := localInfo.ModTime().Sub(remoteInfo.ModTime)
		if diff < 0 {
			diff = -diff
		}
		return diff > 2*time.Second

	case CompareByHash:
		// This is expensive, should be used carefully
		return true // Would need actual hash comparison

	default:
		return true
	}
}

func (s *Syncer) analyzeUpload(localDir, remoteDir string, localMap map[string]os.FileInfo, remoteMap map[string]protocol.FileInfo) []SyncAction {
	var actions []SyncAction

	for relPath, localInfo := range localMap {
		localPath := filepath.Join(localDir, relPath)
		remotePath := filepath.Join(remoteDir, relPath)

		if remoteInfo, exists := remoteMap[relPath]; exists {
			if s.needsSync(localInfo, remoteInfo) && localInfo.ModTime().After(remoteInfo.ModTime) {
				actions = append(actions, SyncAction{
					Type:       "upload",
					LocalPath:  localPath,
					RemotePath: remotePath,
					Reason:     "local file is newer",
				})
			} else {
				actions = append(actions, SyncAction{
					Type:       "skip",
					LocalPath:  localPath,
					RemotePath: remotePath,
					Reason:     "files are identical or remote is newer",
				})
			}
		} else {
			actions = append(actions, SyncAction{
				Type:       "upload",
				LocalPath:  localPath,
				RemotePath: remotePath,
				Reason:     "file does not exist on remote",
			})
		}
	}

	return actions
}

func (s *Syncer) analyzeDownload(localDir, remoteDir string, localMap map[string]os.FileInfo, remoteMap map[string]protocol.FileInfo) []SyncAction {
	var actions []SyncAction

	for relPath, remoteInfo := range remoteMap {
		localPath := filepath.Join(localDir, relPath)
		remotePath := filepath.Join(remoteDir, relPath)

		if localInfo, exists := localMap[relPath]; exists {
			if s.needsSync(localInfo, remoteInfo) && remoteInfo.ModTime.After(localInfo.ModTime()) {
				actions = append(actions, SyncAction{
					Type:       "download",
					LocalPath:  localPath,
					RemotePath: remotePath,
					Reason:     "remote file is newer",
				})
			} else {
				actions = append(actions, SyncAction{
					Type:       "skip",
					LocalPath:  localPath,
					RemotePath: remotePath,
					Reason:     "files are identical or local is newer",
				})
			}
		} else {
			actions = append(actions, SyncAction{
				Type:       "download",
				LocalPath:  localPath,
				RemotePath: remotePath,
				Reason:     "file does not exist locally",
			})
		}
	}

	return actions
}

func (s *Syncer) analyzeMirror(localDir, remoteDir string, localMap map[string]os.FileInfo, remoteMap map[string]protocol.FileInfo) []SyncAction {
	actions := s.analyzeUpload(localDir, remoteDir, localMap, remoteMap)

	// Add deletions for files on remote that don't exist locally
	if s.options.DeleteExtra {
		for relPath := range remoteMap {
			if _, exists := localMap[relPath]; !exists {
				remotePath := filepath.Join(remoteDir, relPath)
				actions = append(actions, SyncAction{
					Type:       "delete_remote",
					RemotePath: remotePath,
					Reason:     "file does not exist locally",
				})
			}
		}
	}

	return actions
}

func (s *Syncer) analyzeBidirectional(localDir, remoteDir string, localMap map[string]os.FileInfo, remoteMap map[string]protocol.FileInfo) []SyncAction {
	var actions []SyncAction

	// Process local files
	for relPath, localInfo := range localMap {
		localPath := filepath.Join(localDir, relPath)
		remotePath := filepath.Join(remoteDir, relPath)

		if remoteInfo, exists := remoteMap[relPath]; exists {
			if s.needsSync(localInfo, remoteInfo) {
				if localInfo.ModTime().After(remoteInfo.ModTime) {
					actions = append(actions, SyncAction{
						Type:       "upload",
						LocalPath:  localPath,
						RemotePath: remotePath,
						Reason:     "local file is newer",
					})
				} else {
					actions = append(actions, SyncAction{
						Type:       "download",
						LocalPath:  localPath,
						RemotePath: remotePath,
						Reason:     "remote file is newer",
					})
				}
			}
		} else {
			actions = append(actions, SyncAction{
				Type:       "upload",
				LocalPath:  localPath,
				RemotePath: remotePath,
				Reason:     "file does not exist on remote",
			})
		}
	}

	// Process remote-only files
	for relPath := range remoteMap {
		if _, exists := localMap[relPath]; !exists {
			localPath := filepath.Join(localDir, relPath)
			remotePath := filepath.Join(remoteDir, relPath)
			actions = append(actions, SyncAction{
				Type:       "download",
				LocalPath:  localPath,
				RemotePath: remotePath,
				Reason:     "file does not exist locally",
			})
		}
	}

	return actions
}

// ComputeLocalChecksum computes MD5 checksum of a local file.
func ComputeLocalChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
