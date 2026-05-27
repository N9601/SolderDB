package realtime

import (
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("coll:notes")
	defer unsub()

	go h.Publish("coll:notes", Event{Kind: EventCreate, Collection: "notes", ID: "abc"})

	select {
	case evt := <-ch:
		if evt.ID != "abc" {
			t.Fatalf("expected id abc, got %s", evt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestParentTopicReceivesChildEvents(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("coll:notes")
	defer unsub()

	go h.Publish("coll:notes:abc", Event{Kind: EventUpdate, Collection: "notes", ID: "abc"})

	select {
	case evt := <-ch:
		if evt.Kind != EventUpdate {
			t.Fatalf("kind: %s", evt.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("parent topic didn't receive child event")
	}
}

func TestNonMatchingTopicIgnored(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("coll:notes")
	defer unsub()

	go h.Publish("coll:other", Event{Kind: EventCreate})

	select {
	case <-ch:
		t.Fatal("should not have received event from a different collection")
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("kv:*")
	unsub()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel should have closed")
	}
}

func TestWildcardTopic(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("kv:*")
	defer unsub()

	go h.Publish("kv:user:1", Event{Kind: EventCreate, Key: "user:1"})

	select {
	case evt := <-ch:
		if evt.Key != "user:1" {
			t.Fatalf("got key %s", evt.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("wildcard didn't match")
	}
}
