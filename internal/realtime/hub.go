// Package realtime implements an in-process pub/sub hub used to fan out
// change events to subscribed clients (HTTP SSE endpoints, UI listeners, etc.).
//
// Topic conventions:
//
//	coll:<name>            , every event on a collection (create/update/delete)
//	coll:<name>:<id>       , events for a single record
//	kv:*                   , every raw KV write
//
// The hub is fire-and-forget. If a subscriber's buffered channel is full,
// the event is dropped for that subscriber only, slow consumers never
// block writers.
package realtime

import (
	"sync"
	"sync/atomic"
)

type EventKind string

const (
	EventCreate EventKind = "create"
	EventUpdate EventKind = "update"
	EventDelete EventKind = "delete"
)

type Event struct {
	Kind       EventKind   `json:"kind"`
	Collection string      `json:"collection,omitempty"`
	Key        string      `json:"key,omitempty"`
	ID         string      `json:"id,omitempty"`
	Data       interface{} `json:"data,omitempty"`
	Timestamp  string      `json:"timestamp"`
}

type subscription struct {
	id     uint64
	topics map[string]struct{}
	ch     chan Event
}

type Hub struct {
	mu        sync.RWMutex
	nextID    uint64
	subs      map[uint64]*subscription
	bufferLen int
}

func NewHub() *Hub {
	return &Hub{
		subs:      make(map[uint64]*subscription),
		bufferLen: 64,
	}
}

// Subscribe registers a subscriber for the given topics and returns the event
// channel + an unsubscribe function. The channel is closed when the subscriber
// is removed; consumers should treat a closed channel as termination.
func (h *Hub) Subscribe(topics ...string) (<-chan Event, func()) {
	id := atomic.AddUint64(&h.nextID, 1)
	set := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		set[t] = struct{}{}
	}
	sub := &subscription{
		id:     id,
		topics: set,
		ch:     make(chan Event, h.bufferLen),
	}
	h.mu.Lock()
	h.subs[id] = sub
	h.mu.Unlock()
	return sub.ch, func() { h.unsubscribe(id) }
}

func (h *Hub) unsubscribe(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.subs[id]; ok {
		close(s.ch)
		delete(h.subs, id)
	}
}

// Publish dispatches an event to every subscriber whose topic set matches.
// A subscriber matches when:
//   - it subscribes to the literal topic; OR
//   - its topic ends with "*" and the event topic has the same prefix; OR
//   - it subscribes to a parent topic and the event topic begins with
//     parent + ":"  (so coll:notes receives coll:notes:<id> events too)
func (h *Hub) Publish(topic string, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subs {
		if !matches(sub.topics, topic) {
			continue
		}
		select {
		case sub.ch <- evt:
		default:
			// Slow consumer, drop the event for them only.
		}
	}
}

func matches(topics map[string]struct{}, topic string) bool {
	if _, ok := topics[topic]; ok {
		return true
	}
	for t := range topics {
		if t == "*" {
			return true
		}
		if len(t) > 1 && t[len(t)-1] == '*' && len(topic) >= len(t)-1 && topic[:len(t)-1] == t[:len(t)-1] {
			return true
		}
		if len(topic) > len(t) && topic[:len(t)] == t && topic[len(t)] == ':' {
			return true
		}
	}
	return false
}

// SubscriberCount is useful for stats / debugging.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
