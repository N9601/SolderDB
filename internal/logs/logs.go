// Package logs implements an in-memory ring buffer of recent API requests
// so the admin UI can render an activity feed. Entries are also published
// to the realtime hub under the "logs:*" topic for live tailing.
package logs

import (
	"sync"
	"time"
)

type Entry struct {
	Timestamp string `json:"timestamp"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	DurationMs int64 `json:"durationMs"`
	User      string `json:"user,omitempty"`
	Remote    string `json:"remote,omitempty"`
}

// Publisher is satisfied by realtime.Hub — kept narrow so logs doesn't
// import realtime directly.
type Publisher interface {
	Publish(topic string, evt interface{})
}

type Buffer struct {
	mu       sync.RWMutex
	entries  []Entry
	max      int
	nextSeq  uint64
	publish  func(Entry)
}

func New(max int) *Buffer {
	if max <= 0 {
		max = 500
	}
	return &Buffer{entries: make([]Entry, 0, max), max: max}
}

// SetPublisher installs a callback invoked once per appended entry. Use this
// to bridge the buffer to the realtime hub.
func (b *Buffer) SetPublisher(fn func(Entry)) {
	b.mu.Lock()
	b.publish = fn
	b.mu.Unlock()
}

// Append records a new entry; oldest is dropped if the buffer is full.
func (b *Buffer) Append(e Entry) {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b.mu.Lock()
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		// drop the oldest in O(n) — acceptable for hundreds of entries.
		b.entries = b.entries[len(b.entries)-b.max:]
	}
	b.nextSeq++
	pub := b.publish
	b.mu.Unlock()
	if pub != nil {
		pub(e)
	}
}

// Tail returns up to `limit` newest entries, newest-first.
func (b *Buffer) Tail(limit int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.entries) {
		limit = len(b.entries)
	}
	out := make([]Entry, limit)
	for i := 0; i < limit; i++ {
		out[i] = b.entries[len(b.entries)-1-i]
	}
	return out
}

func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.entries)
}
