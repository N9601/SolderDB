package solderdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubscribeReportsRefusedStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"authentication required"}`)
	}))
	defer srv.Close()

	c := New(srv.URL)
	stop, err := c.Collection("notes").Subscribe(context.Background(), func(Event) {
		t.Error("handler called for a refused stream")
	})
	if stop == nil {
		t.Fatal("stop must never be nil")
	}
	stop()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || apiErr.Message != "authentication required" {
		t.Fatalf("expected a 401 APIError, got %v", err)
	}
}

func TestSubscribeDeliversEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("topic"); got != "coll:notes" {
			t.Errorf("topic=%q, want coll:notes", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\nevent: create\ndata: {\"kind\":\"create\",\"collection\":\"notes\",\"id\":\"r1\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	got := make(chan Event, 1)
	stop, err := New(srv.URL).Collection("notes").Subscribe(context.Background(), func(evt Event) {
		got <- evt
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stop()

	select {
	case evt := <-got:
		if evt.Kind != "create" || evt.ID != "r1" {
			t.Fatalf("unexpected event %+v", evt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event delivered")
	}
}
