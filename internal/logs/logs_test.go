package logs

import "testing"

func TestRingBufferDropsOldest(t *testing.T) {
	b := New(3)
	for i := 0; i < 5; i++ {
		b.Append(Entry{Path: "p", Status: 200, DurationMs: int64(i)})
	}
	if b.Len() != 3 {
		t.Fatalf("expected buffer length 3, got %d", b.Len())
	}
	tail := b.Tail(10)
	if len(tail) != 3 {
		t.Fatalf("expected tail 3, got %d", len(tail))
	}
	// Newest-first: tail[0] should have the largest duration.
	if tail[0].DurationMs != 4 || tail[2].DurationMs != 2 {
		t.Fatalf("unexpected tail order: %+v", tail)
	}
}

func TestPublisherFires(t *testing.T) {
	b := New(10)
	var got Entry
	b.SetPublisher(func(e Entry) { got = e })
	b.Append(Entry{Path: "/x", Status: 201})
	if got.Path != "/x" || got.Status != 201 {
		t.Fatalf("publisher not invoked properly: %+v", got)
	}
}
