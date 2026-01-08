// Package ui provides directory listing cache for improved performance.
package ui

import (
	"sync"
	"time"
)

// CacheEntry represents a cached directory listing.
type CacheEntry struct {
	Files     []FileItem
	Timestamp time.Time
}

// DirCache provides a time-limited cache for directory listings.
type DirCache struct {
	entries map[string]*CacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	done    chan struct{}
}

// NewDirCache creates a new directory cache with the specified TTL.
func NewDirCache(ttl time.Duration) *DirCache {
	cache := &DirCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a cached directory listing if available and not expired.
func (c *DirCache) Get(path string) ([]FileItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[path]
	if !exists {
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	// Return a copy to prevent modification
	files := make([]FileItem, len(entry.Files))
	copy(files, entry.Files)
	return files, true
}

// Set stores a directory listing in the cache.
func (c *DirCache) Set(path string, files []FileItem) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store a copy to prevent external modification
	filesCopy := make([]FileItem, len(files))
	copy(filesCopy, files)

	c.entries[path] = &CacheEntry{
		Files:     filesCopy,
		Timestamp: time.Now(),
	}
}

// Invalidate removes a specific path from the cache.
func (c *DirCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
}

// InvalidateAll clears the entire cache.
func (c *DirCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
}

// cleanupLoop periodically removes expired entries.
func (c *DirCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.done:
			return
		}
	}
}

// Close stops the cleanup goroutine and releases resources.
func (c *DirCache) Close() {
	close(c.done)
}

// cleanup removes expired entries from the cache.
func (c *DirCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for path, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, path)
		}
	}
}

// Size returns the number of cached entries.
func (c *DirCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// DefaultCacheTTL is the default cache time-to-live.
const DefaultCacheTTL = 30 * time.Second
