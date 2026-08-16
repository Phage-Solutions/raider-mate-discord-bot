package client

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestStreamNotificationsCallsBackOncePerEvent(t *testing.T) {
	var gotAuth, gotAccept string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Two events around a heartbeat comment, which must not count as one.
		if _, err := w.Write([]byte("event: notification\ndata: {}\n\n:\n\nevent: notification\ndata: {}\n\n")); err != nil {
			t.Errorf("writing stream: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events := 0
	if err := c.StreamNotifications(ctx, func() { events++ }); err != nil {
		t.Fatalf("streaming: %v", err)
	}

	if events != 2 {
		t.Errorf("events = %d, want 2 (the heartbeat is not one)", events)
	}
	if gotAuth != "Bearer "+testKey {
		t.Errorf("Authorization = %q, want the service key", gotAuth)
	}
	if gotAccept != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", gotAccept)
	}
}

func TestStreamNotificationsReportsARefusedStream(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	// The caller redials on any error, so a bad key must surface as one rather than
	// looking like a stream that simply ended.
	if err := c.StreamNotifications(context.Background(), func() {
		t.Error("callback fired on a refused stream")
	}); err == nil {
		t.Fatal("StreamNotifications() error = nil, want an error for 401")
	}
}

func TestStreamNotificationsStopsWithTheContext(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		http.NewResponseController(w).Flush() //nolint:errcheck
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.StreamNotifications(ctx, func() {}) }()

	// Shutdown cancels the context the stream goroutine runs under, and the bot cannot
	// exit while this call is still holding a connection open.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StreamNotifications did not return after the context was cancelled")
	}
}
