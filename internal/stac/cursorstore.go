package stac

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// CursorStore defines the interface for storing and retrieving pagination cursors.
// Server-side storage is used when cursors are too large to fit in URL parameters.
type CursorStore interface {
	// Store saves a cursor and returns a short token to reference it
	Store(cursor *Cursor) (token string, err error)

	// Retrieve gets a cursor by its token
	Retrieve(token string) (*Cursor, error)

	// Delete removes a cursor (optional cleanup)
	Delete(token string) error
}

// cursorEntry holds a cursor with its expiration time
type cursorEntry struct {
	cursor    *Cursor
	expiresAt time.Time
}

// MemoryCursorStore implements CursorStore using in-memory storage with TTL.
// This is suitable for single-instance deployments. For distributed deployments,
// use Redis or another shared storage backend.
type MemoryCursorStore struct {
	mu       sync.RWMutex
	cursors  map[string]cursorEntry
	ttl      time.Duration
	stopChan chan struct{}
}

// NewMemoryCursorStore creates a new in-memory cursor store.
// ttl specifies how long cursors are kept before expiration.
// cleanupInterval specifies how often to run the cleanup routine.
func NewMemoryCursorStore(ttl time.Duration, cleanupInterval time.Duration) *MemoryCursorStore {
	store := &MemoryCursorStore{
		cursors:  make(map[string]cursorEntry),
		ttl:      ttl,
		stopChan: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go store.cleanupLoop(cleanupInterval)

	return store
}

// Store saves a cursor and returns a short token.
func (s *MemoryCursorStore) Store(cursor *Cursor) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cursors[token] = cursorEntry{
		cursor:    cursor,
		expiresAt: time.Now().Add(s.ttl),
	}

	return token, nil
}

// Retrieve gets a cursor by its token.
func (s *MemoryCursorStore) Retrieve(token string) (*Cursor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.cursors[token]
	if !exists {
		return nil, ErrCursorNotFound
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, ErrCursorExpired
	}

	return entry.cursor, nil
}

// Delete removes a cursor by its token.
func (s *MemoryCursorStore) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cursors, token)
	return nil
}

// Stop stops the background cleanup goroutine.
func (s *MemoryCursorStore) Stop() {
	close(s.stopChan)
}

// cleanupLoop periodically removes expired cursors.
func (s *MemoryCursorStore) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopChan:
			return
		}
	}
}

// cleanup removes all expired cursors.
func (s *MemoryCursorStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, entry := range s.cursors {
		if now.After(entry.expiresAt) {
			delete(s.cursors, token)
		}
	}
}

// Stats returns statistics about the cursor store.
func (s *MemoryCursorStore) Stats() (count int, oldestAge time.Duration) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count = len(s.cursors)
	if count == 0 {
		return 0, 0
	}

	var oldest time.Time
	now := time.Now()
	for _, entry := range s.cursors {
		created := entry.expiresAt.Add(-s.ttl)
		if oldest.IsZero() || created.Before(oldest) {
			oldest = created
		}
	}

	return count, now.Sub(oldest)
}

// generateToken creates a cryptographically secure random token.
func generateToken() (string, error) {
	bytes := make([]byte, 16) // 128 bits = 32 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Sentinel errors for cursor store operations
var (
	ErrCursorNotFound = cursorStoreError("cursor not found")
	ErrCursorExpired  = cursorStoreError("cursor expired")
)

type cursorStoreError string

func (e cursorStoreError) Error() string {
	return string(e)
}
