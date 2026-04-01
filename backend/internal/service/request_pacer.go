package service

import (
	"math/rand"
	"sync"
	"time"
)

// RequestPacer adds small random delays between rapid consecutive requests
// to the same account, making the request pattern more realistic.
// Real Claude Code users have variable think-time between requests.
//
// This is NOT a rate limiter — it's a pattern normalizer. It only adds delay
// when requests come faster than minInterval (suggesting automated usage).
//
// Starts an internal cleanup goroutine. Call Stop() to release resources.
type RequestPacer struct {
	mu          sync.Mutex
	lastRequest map[int64]time.Time // accountID -> last request time
	rng         *rand.Rand
	stopCh      chan struct{}

	// minInterval is the minimum time between requests before pacing kicks in.
	// Requests slower than this pass through without delay.
	minInterval time.Duration

	// maxJitter is the maximum random delay added when pacing is needed.
	maxJitter time.Duration
}

// NewRequestPacer creates a new RequestPacer and starts a background cleanup goroutine.
// minInterval: requests faster than this get paced (recommended: 1-2s)
// maxJitter: maximum random delay added (recommended: 2-4s)
func NewRequestPacer(minInterval, maxJitter time.Duration) *RequestPacer {
	p := &RequestPacer{
		lastRequest: make(map[int64]time.Time, 64),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		minInterval: minInterval,
		maxJitter:   maxJitter,
		stopCh:      make(chan struct{}),
	}
	go p.cleanupLoop()
	return p
}

// Stop halts the background cleanup goroutine.
func (p *RequestPacer) Stop() {
	select {
	case <-p.stopCh:
		// already stopped
	default:
		close(p.stopCh)
	}
}

// cleanupLoop periodically removes stale entries to prevent unbounded map growth.
func (p *RequestPacer) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.cleanup(10 * time.Minute)
		}
	}
}

// GetDelay returns the delay that should be applied before sending the request.
// Returns 0 if no pacing is needed.
func (p *RequestPacer) GetDelay(accountID int64) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	last, exists := p.lastRequest[accountID]
	p.lastRequest[accountID] = now

	if !exists {
		return 0
	}

	elapsed := now.Sub(last)
	if elapsed >= p.minInterval {
		return 0 // Enough time has passed, no pacing needed
	}

	// Add random jitter: minInterval - elapsed + random(0, maxJitter)
	remaining := p.minInterval - elapsed
	jitter := time.Duration(p.rng.Int63n(int64(p.maxJitter)))
	return remaining + jitter
}

// cleanup removes stale entries older than maxAge.
func (p *RequestPacer) cleanup(maxAge time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, t := range p.lastRequest {
		if t.Before(cutoff) {
			delete(p.lastRequest, id)
		}
	}
}
