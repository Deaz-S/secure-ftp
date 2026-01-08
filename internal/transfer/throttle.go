// Package transfer provides bandwidth throttling for file transfers.
package transfer

import (
	"io"
	"sync"
	"time"
)

// ThrottledReader wraps an io.Reader with bandwidth limiting.
type ThrottledReader struct {
	reader      io.Reader
	limiter     *RateLimiter
}

// ThrottledWriter wraps an io.Writer with bandwidth limiting.
type ThrottledWriter struct {
	writer      io.Writer
	limiter     *RateLimiter
}

// RateLimiter controls the rate of data transfer.
type RateLimiter struct {
	bytesPerSecond int64
	mu             sync.Mutex
	tokens         int64
	lastRefill     time.Time
}

// NewRateLimiter creates a new rate limiter.
// bytesPerSecond of 0 means unlimited.
func NewRateLimiter(bytesPerSecond int64) *RateLimiter {
	return &RateLimiter{
		bytesPerSecond: bytesPerSecond,
		tokens:         bytesPerSecond,
		lastRefill:     time.Now(),
	}
}

// SetRate updates the rate limit.
func (r *RateLimiter) SetRate(bytesPerSecond int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bytesPerSecond = bytesPerSecond
	r.tokens = bytesPerSecond
}

// GetRate returns the current rate limit.
func (r *RateLimiter) GetRate() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytesPerSecond
}

// Wait blocks until n bytes can be transferred.
func (r *RateLimiter) Wait(n int64) {
	if r.bytesPerSecond <= 0 {
		return // Unlimited
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	refill := int64(elapsed.Seconds() * float64(r.bytesPerSecond))
	r.tokens += refill
	if r.tokens > r.bytesPerSecond {
		r.tokens = r.bytesPerSecond
	}
	r.lastRefill = now

	// If we don't have enough tokens, wait
	for r.tokens < n {
		// Calculate wait time
		needed := n - r.tokens
		waitTime := time.Duration(float64(needed) / float64(r.bytesPerSecond) * float64(time.Second))

		r.mu.Unlock()
		time.Sleep(waitTime)
		r.mu.Lock()

		// Refill after waiting
		now = time.Now()
		elapsed = now.Sub(r.lastRefill)
		refill = int64(elapsed.Seconds() * float64(r.bytesPerSecond))
		r.tokens += refill
		if r.tokens > r.bytesPerSecond {
			r.tokens = r.bytesPerSecond
		}
		r.lastRefill = now
	}

	// Consume tokens
	r.tokens -= n
}

// NewThrottledReader creates a new throttled reader.
func NewThrottledReader(reader io.Reader, limiter *RateLimiter) *ThrottledReader {
	return &ThrottledReader{
		reader:  reader,
		limiter: limiter,
	}
}

// Read implements io.Reader with rate limiting.
func (tr *ThrottledReader) Read(p []byte) (int, error) {
	n, err := tr.reader.Read(p)
	if n > 0 && tr.limiter != nil {
		tr.limiter.Wait(int64(n))
	}
	return n, err
}

// NewThrottledWriter creates a new throttled writer.
func NewThrottledWriter(writer io.Writer, limiter *RateLimiter) *ThrottledWriter {
	return &ThrottledWriter{
		writer:  writer,
		limiter: limiter,
	}
}

// Write implements io.Writer with rate limiting.
func (tw *ThrottledWriter) Write(p []byte) (int, error) {
	if tw.limiter != nil {
		tw.limiter.Wait(int64(len(p)))
	}
	return tw.writer.Write(p)
}

// BandwidthLimiter manages bandwidth limits for uploads and downloads.
type BandwidthLimiter struct {
	uploadLimiter   *RateLimiter
	downloadLimiter *RateLimiter
}

// NewBandwidthLimiter creates a new bandwidth limiter.
// Rates of 0 mean unlimited.
func NewBandwidthLimiter(uploadBytesPerSec, downloadBytesPerSec int64) *BandwidthLimiter {
	return &BandwidthLimiter{
		uploadLimiter:   NewRateLimiter(uploadBytesPerSec),
		downloadLimiter: NewRateLimiter(downloadBytesPerSec),
	}
}

// SetUploadRate sets the upload rate limit.
func (bl *BandwidthLimiter) SetUploadRate(bytesPerSecond int64) {
	bl.uploadLimiter.SetRate(bytesPerSecond)
}

// SetDownloadRate sets the download rate limit.
func (bl *BandwidthLimiter) SetDownloadRate(bytesPerSecond int64) {
	bl.downloadLimiter.SetRate(bytesPerSecond)
}

// GetUploadRate returns the upload rate limit.
func (bl *BandwidthLimiter) GetUploadRate() int64 {
	return bl.uploadLimiter.GetRate()
}

// GetDownloadRate returns the download rate limit.
func (bl *BandwidthLimiter) GetDownloadRate() int64 {
	return bl.downloadLimiter.GetRate()
}

// WrapReader wraps a reader for download throttling.
func (bl *BandwidthLimiter) WrapReader(r io.Reader) io.Reader {
	if bl.downloadLimiter.GetRate() <= 0 {
		return r
	}
	return NewThrottledReader(r, bl.downloadLimiter)
}

// WrapWriter wraps a writer for upload throttling.
func (bl *BandwidthLimiter) WrapWriter(w io.Writer) io.Writer {
	if bl.uploadLimiter.GetRate() <= 0 {
		return w
	}
	return NewThrottledWriter(w, bl.uploadLimiter)
}

// Common bandwidth presets (bytes per second)
const (
	BandwidthUnlimited = 0
	Bandwidth100Kbps   = 12500     // 100 Kbit/s
	Bandwidth256Kbps   = 32000     // 256 Kbit/s
	Bandwidth512Kbps   = 64000     // 512 Kbit/s
	Bandwidth1Mbps     = 125000    // 1 Mbit/s
	Bandwidth2Mbps     = 250000    // 2 Mbit/s
	Bandwidth5Mbps     = 625000    // 5 Mbit/s
	Bandwidth10Mbps    = 1250000   // 10 Mbit/s
	Bandwidth50Mbps    = 6250000   // 50 Mbit/s
	Bandwidth100Mbps   = 12500000  // 100 Mbit/s
)

// BandwidthPreset represents a bandwidth preset with name and rate.
type BandwidthPreset struct {
	Name           string
	BytesPerSecond int64
}

// GetBandwidthPresets returns available bandwidth presets.
func GetBandwidthPresets() []BandwidthPreset {
	return []BandwidthPreset{
		{"Unlimited", BandwidthUnlimited},
		{"100 Kbit/s", Bandwidth100Kbps},
		{"256 Kbit/s", Bandwidth256Kbps},
		{"512 Kbit/s", Bandwidth512Kbps},
		{"1 Mbit/s", Bandwidth1Mbps},
		{"2 Mbit/s", Bandwidth2Mbps},
		{"5 Mbit/s", Bandwidth5Mbps},
		{"10 Mbit/s", Bandwidth10Mbps},
		{"50 Mbit/s", Bandwidth50Mbps},
		{"100 Mbit/s", Bandwidth100Mbps},
	}
}
