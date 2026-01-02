// Package transfer provides transfer resumption functionality.
package transfer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ResumeInfo stores information about an incomplete transfer for resumption.
type ResumeInfo struct {
	ID             string            `json:"id"`
	Direction      TransferDirection `json:"direction"`
	LocalPath      string            `json:"local_path"`
	RemotePath     string            `json:"remote_path"`
	TotalBytes     int64             `json:"total_bytes"`
	TransferredBytes int64           `json:"transferred_bytes"`
	StartTime      time.Time         `json:"start_time"`
	LastUpdate     time.Time         `json:"last_update"`
	Checksum       string            `json:"checksum,omitempty"`
}

// ResumeManager manages transfer resumption state.
type ResumeManager struct {
	statePath string
	transfers map[string]*ResumeInfo
	mu        sync.RWMutex
}

// NewResumeManager creates a new resume manager.
func NewResumeManager(statePath string) (*ResumeManager, error) {
	rm := &ResumeManager{
		statePath: statePath,
		transfers: make(map[string]*ResumeInfo),
	}

	// Load existing state
	if err := rm.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return rm, nil
}

// load reads the resume state from disk.
func (rm *ResumeManager) load() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	data, err := os.ReadFile(rm.statePath)
	if err != nil {
		return err
	}

	var transfers []*ResumeInfo
	if err := json.Unmarshal(data, &transfers); err != nil {
		return err
	}

	rm.transfers = make(map[string]*ResumeInfo)
	for _, t := range transfers {
		rm.transfers[t.ID] = t
	}

	return nil
}

// save writes the resume state to disk.
func (rm *ResumeManager) save() error {
	rm.mu.RLock()
	transfers := make([]*ResumeInfo, 0, len(rm.transfers))
	for _, t := range rm.transfers {
		transfers = append(transfers, t)
	}
	rm.mu.RUnlock()

	data, err := json.MarshalIndent(transfers, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(rm.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(rm.statePath, data, 0644)
}

// StartTransfer records the start of a new transfer.
func (rm *ResumeManager) StartTransfer(id string, direction TransferDirection, localPath, remotePath string, totalBytes int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.transfers[id] = &ResumeInfo{
		ID:             id,
		Direction:      direction,
		LocalPath:      localPath,
		RemotePath:     remotePath,
		TotalBytes:     totalBytes,
		TransferredBytes: 0,
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	go rm.save()
}

// UpdateProgress updates the progress of an ongoing transfer.
func (rm *ResumeManager) UpdateProgress(id string, transferredBytes int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if info, ok := rm.transfers[id]; ok {
		info.TransferredBytes = transferredBytes
		info.LastUpdate = time.Now()

		// Save periodically (not on every update to avoid disk thrashing)
		go rm.save()
	}
}

// CompleteTransfer marks a transfer as completed and removes it from resume state.
func (rm *ResumeManager) CompleteTransfer(id string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.transfers, id)
	go rm.save()
}

// FailTransfer marks a transfer as failed but keeps it for potential resumption.
func (rm *ResumeManager) FailTransfer(id string, transferredBytes int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if info, ok := rm.transfers[id]; ok {
		info.TransferredBytes = transferredBytes
		info.LastUpdate = time.Now()
		go rm.save()
	}
}

// GetIncomplete returns all incomplete transfers that can be resumed.
func (rm *ResumeManager) GetIncomplete() []*ResumeInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*ResumeInfo, 0, len(rm.transfers))
	for _, info := range rm.transfers {
		// Only include transfers that have progress and are not complete
		if info.TransferredBytes > 0 && info.TransferredBytes < info.TotalBytes {
			result = append(result, info)
		}
	}

	return result
}

// GetResumeInfo returns resume info for a specific transfer.
func (rm *ResumeManager) GetResumeInfo(id string) *ResumeInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if info, ok := rm.transfers[id]; ok {
		return info
	}
	return nil
}

// ClearOld removes resume entries older than the specified duration.
func (rm *ResumeManager) ClearOld(maxAge time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, info := range rm.transfers {
		if info.LastUpdate.Before(cutoff) {
			delete(rm.transfers, id)
		}
	}

	go rm.save()
}

// Clear removes all resume entries.
func (rm *ResumeManager) Clear() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.transfers = make(map[string]*ResumeInfo)
	go rm.save()
}

// Count returns the number of incomplete transfers.
func (rm *ResumeManager) Count() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.transfers)
}
