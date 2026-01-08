// Package transfer provides file transfer management with queue support.
package transfer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"secure-ftp/internal/protocol"
	"secure-ftp/pkg/logger"
)

// TransferDirection indicates upload or download.
type TransferDirection int

const (
	DirectionUpload TransferDirection = iota
	DirectionDownload
)

// TransferStatus represents the current state of a transfer.
type TransferStatus int

const (
	StatusPending TransferStatus = iota
	StatusInProgress
	StatusCompleted
	StatusFailed
	StatusCancelled
	StatusPaused
)

func (s TransferStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusInProgress:
		return "In Progress"
	case StatusCompleted:
		return "Completed"
	case StatusFailed:
		return "Failed"
	case StatusCancelled:
		return "Cancelled"
	case StatusPaused:
		return "Paused"
	default:
		return "Unknown"
	}
}

// TransferItem represents a single transfer task.
type TransferItem struct {
	ID             string
	Direction      TransferDirection
	LocalPath      string
	RemotePath     string
	TotalBytes     int64
	TransferredBytes int64
	BytesPerSecond int64
	Status         TransferStatus
	Error          error
	StartTime      time.Time
	EndTime        time.Time
	Priority       int // Higher = more priority

	ctx    context.Context
	cancel context.CancelFunc
}

// Progress returns the transfer progress as a percentage.
func (t *TransferItem) Progress() float64 {
	if t.TotalBytes == 0 {
		return 0
	}
	return float64(t.TransferredBytes) / float64(t.TotalBytes) * 100
}

// RemainingTime returns the estimated remaining time.
func (t *TransferItem) RemainingTime() time.Duration {
	if t.BytesPerSecond == 0 {
		return 0
	}
	remaining := t.TotalBytes - t.TransferredBytes
	return time.Duration(remaining/t.BytesPerSecond) * time.Second
}

// TransferManager manages file transfers with a concurrent queue.
type TransferManager struct {
	client      protocol.Protocol
	queue       []*TransferItem
	history     []*TransferItem
	maxParallel int
	active      int
	mu          sync.RWMutex
	log         *logger.Logger

	onUpdate   func(*TransferItem)
	onComplete func(*TransferItem)

	idCounter int
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewTransferManager creates a new transfer manager.
func NewTransferManager(client protocol.Protocol, maxParallel int) *TransferManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TransferManager{
		client:      client,
		queue:       make([]*TransferItem, 0),
		history:     make([]*TransferItem, 0),
		maxParallel: maxParallel,
		log:         logger.GetInstance(),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetUpdateCallback sets the callback for transfer updates.
func (m *TransferManager) SetUpdateCallback(fn func(*TransferItem)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onUpdate = fn
}

// SetCompleteCallback sets the callback for transfer completion.
func (m *TransferManager) SetCompleteCallback(fn func(*TransferItem)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onComplete = fn
}

// AddUpload queues an upload task.
func (m *TransferManager) AddUpload(localPath, remotePath string, priority int) *TransferItem {
	return m.addTransfer(DirectionUpload, localPath, remotePath, priority)
}

// AddDownload queues a download task.
func (m *TransferManager) AddDownload(remotePath, localPath string, priority int) *TransferItem {
	return m.addTransfer(DirectionDownload, localPath, remotePath, priority)
}

func (m *TransferManager) addTransfer(direction TransferDirection, localPath, remotePath string, priority int) *TransferItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idCounter++
	ctx, cancel := context.WithCancel(m.ctx)

	item := &TransferItem{
		ID:         fmt.Sprintf("transfer-%d", m.idCounter),
		Direction:  direction,
		LocalPath:  localPath,
		RemotePath: remotePath,
		Status:     StatusPending,
		Priority:   priority,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Insert in priority order
	inserted := false
	for i, existing := range m.queue {
		if item.Priority > existing.Priority {
			m.queue = append(m.queue[:i], append([]*TransferItem{item}, m.queue[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		m.queue = append(m.queue, item)
	}

	// Try to start more transfers
	go m.processQueue()

	return item
}

// processQueue starts pending transfers if slots are available.
func (m *TransferManager) processQueue() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for m.active < m.maxParallel {
		// Find next pending item
		var item *TransferItem
		for _, q := range m.queue {
			if q.Status == StatusPending {
				item = q
				break
			}
		}

		if item == nil {
			break
		}

		m.active++
		item.Status = StatusInProgress
		item.StartTime = time.Now()

		m.wg.Add(1)
		go m.executeTransfer(item)
	}
}

// executeTransfer performs the actual file transfer.
func (m *TransferManager) executeTransfer(item *TransferItem) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		m.active--
		m.mu.Unlock()
		go m.processQueue()
	}()

	progressFn := func(progress protocol.TransferProgress) {
		item.TotalBytes = progress.TotalBytes
		item.TransferredBytes = progress.TransferredBytes
		item.BytesPerSecond = progress.BytesPerSecond

		if m.onUpdate != nil {
			m.onUpdate(item)
		}
	}

	var err error
	startTime := time.Now()

	if item.Direction == DirectionUpload {
		err = m.client.Upload(item.ctx, item.LocalPath, item.RemotePath, true, progressFn)
	} else {
		err = m.client.Download(item.ctx, item.RemotePath, item.LocalPath, true, progressFn)
	}

	item.EndTime = time.Now()
	duration := item.EndTime.Sub(startTime)

	if err != nil {
		if item.ctx.Err() == context.Canceled {
			item.Status = StatusCancelled
		} else {
			item.Status = StatusFailed
			item.Error = err
		}
	} else {
		item.Status = StatusCompleted
	}

	// Log transfer
	if m.log != nil {
		direction := "download"
		if item.Direction == DirectionUpload {
			direction = "upload"
		}
		m.log.LogTransfer(direction, m.client.GetProtocolName(), item.LocalPath, item.RemotePath, item.TotalBytes, duration, err)
	}

	// Move to history
	m.mu.Lock()
	m.removeFromQueue(item.ID)
	m.history = append(m.history, item)

	// Keep history limited
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	m.mu.Unlock()

	if m.onComplete != nil {
		m.onComplete(item)
	}
}

// removeFromQueue removes an item from the queue by ID.
func (m *TransferManager) removeFromQueue(id string) {
	for i, item := range m.queue {
		if item.ID == id {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			return
		}
	}
}

// Cancel cancels a transfer.
func (m *TransferManager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.queue {
		if item.ID == id {
			if item.Status == StatusInProgress {
				item.cancel()
			} else {
				item.Status = StatusCancelled
			}
			return nil
		}
	}

	return fmt.Errorf("transfer not found: %s", id)
}

// CancelAll cancels all pending and in-progress transfers.
func (m *TransferManager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.queue {
		if item.Status == StatusPending || item.Status == StatusInProgress {
			item.cancel()
			item.Status = StatusCancelled
		}
	}
}

// Pause pauses a transfer (only pending transfers).
func (m *TransferManager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.queue {
		if item.ID == id && item.Status == StatusPending {
			item.Status = StatusPaused
			return nil
		}
	}

	return fmt.Errorf("cannot pause transfer: %s", id)
}

// Resume resumes a paused transfer.
func (m *TransferManager) Resume(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.queue {
		if item.ID == id && item.Status == StatusPaused {
			item.Status = StatusPending
			go m.processQueue()
			return nil
		}
	}

	return fmt.Errorf("cannot resume transfer: %s", id)
}

// GetQueue returns the current transfer queue.
func (m *TransferManager) GetQueue() []*TransferItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TransferItem, len(m.queue))
	copy(result, m.queue)
	return result
}

// GetHistory returns the transfer history.
func (m *TransferManager) GetHistory() []*TransferItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TransferItem, len(m.history))
	copy(result, m.history)
	return result
}

// GetActiveCount returns the number of active transfers.
func (m *TransferManager) GetActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// SetMaxParallel sets the maximum number of parallel transfers.
func (m *TransferManager) SetMaxParallel(n int) {
	m.mu.Lock()
	m.maxParallel = n
	m.mu.Unlock()
	go m.processQueue()
}

// Wait waits for all transfers to complete.
func (m *TransferManager) Wait() {
	m.wg.Wait()
}

// Stop stops the transfer manager and cancels all transfers.
func (m *TransferManager) Stop() {
	m.cancel()
	m.CancelAll()
	m.wg.Wait()
}

// ClearHistory clears the transfer history.
func (m *TransferManager) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = make([]*TransferItem, 0)
}

// Retry retries a failed transfer by re-adding it to the queue.
func (m *TransferManager) Retry(id string) (*TransferItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Search in history for the failed transfer
	for i, item := range m.history {
		if item.ID == id && item.Status == StatusFailed {
			// Remove from history
			m.history = append(m.history[:i], m.history[i+1:]...)

			// Create new transfer with same params
			m.idCounter++
			ctx, cancel := context.WithCancel(m.ctx)

			newItem := &TransferItem{
				ID:         fmt.Sprintf("transfer-%d", m.idCounter),
				Direction:  item.Direction,
				LocalPath:  item.LocalPath,
				RemotePath: item.RemotePath,
				Status:     StatusPending,
				Priority:   item.Priority,
				ctx:        ctx,
				cancel:     cancel,
			}

			// Add to queue with priority
			inserted := false
			for j, existing := range m.queue {
				if newItem.Priority > existing.Priority {
					m.queue = append(m.queue[:j], append([]*TransferItem{newItem}, m.queue[j:]...)...)
					inserted = true
					break
				}
			}
			if !inserted {
				m.queue = append(m.queue, newItem)
			}

			// Start processing
			go m.processQueue()

			return newItem, nil
		}
	}

	return nil, fmt.Errorf("failed transfer not found: %s", id)
}

// GetItem returns a transfer item by ID from queue or history.
func (m *TransferManager) GetItem(id string) *TransferItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, item := range m.queue {
		if item.ID == id {
			return item
		}
	}

	for _, item := range m.history {
		if item.ID == id {
			return item
		}
	}

	return nil
}
