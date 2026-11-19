package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// frame writes one SSE frame: an id line (when non-empty), the data payload,
// and the terminating blank line.
func frame(id, payload string) string {
	var b strings.Builder
	if id != "" {
		fmt.Fprintf(&b, "id: %s\n", id)
	}
	fmt.Fprintf(&b, "data: %s\n\n", payload)
	return b.String()
}

const (
	payloadA = `{"$schema":"/mediawiki/recentchange/1.0.0",` +
		`"meta":{"domain":"ca.wikipedia.org","id":"evt-A","dt":"2026-11-19T12:00:00Z"},` +
		`"id":1,"type":"edit","title":"Riu","user":"jordi"}`
	payloadB = `{"$schema":"/mediawiki/recentchange/1.0.0",` +
		`"meta":{"domain":"fr.wikipedia.org","id":"evt-B","dt":"2026-11-19T12:00:01Z"},` +
		`"id":2,"type":"edit","title":"Fleuve","user":"claude"}`
)

func tinyBackoff(c *Client) *Client {
	c.base, c.max = 5*time.Millisecond, 20*time.Millisecond
	return c
}

func TestSourceReceivesEvents(t *testing.T) { // R1 (+R10 malformed counting)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ":ok\n\n") // keepalive comment — must be ignored
		fmt.Fprint(w, frame(`{"topic":"t","partition":0,"offset":1}`, payloadA))
		if f := w.(http.Flusher); f != nil {
			f.Flush()
		}
		fmt.Fprint(w, "data: {broken\n\n") // malformed payload — counted, not fatal
		fmt.Fprint(w, frame("", payloadB))
		if f := w.(http.Flusher); f != nil {
			f.Flush()
		}
		<-r.Context().Done() // hold the stream open until the test ends
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := tinyBackoff(New(srv.URL))
	events, errc := c.Stream(ctx)

	first, ok := <-events
	if !ok {
		t.Fatal("events channel closed before first event")
	}
	if first.MetaID != "evt-A" || first.Domain != "ca.wikipedia.org" || first.Title != "Riu" {
		t.Fatalf("first event = %+v", first)
	}
	if got := c.LastEventID(); got != `{"topic":"t","partition":0,"offset":1}` {
		t.Fatalf("LastEventID() = %q", got)
	}

	second := <-events
	if second.MetaID != "evt-B" || second.Domain != "fr.wikipedia.org" {
		t.Fatalf("second event = %+v", second)
	}
	if n := c.InvalidFrames(); n != 1 {
		t.Fatalf("InvalidFrames() = %d, want 1 (the malformed payload)", n)
	}

	cancel()
	assertCloses(t, events, errc)
}

func TestSourceReconnectResume(t *testing.T) { // R2: drop → reconnect → Last-Event-ID
	var conns atomic.Int64
	sawHeader := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch conns.Add(1) {
		case 1:
			fmt.Fprint(w, frame("A", payloadA))
			if f := w.(http.Flusher); f != nil {
				f.Flush()
			}
			// drop the connection without warning — the river must recover
		case 2:
			sawHeader <- r.Header.Get("Last-Event-ID")
			fmt.Fprint(w, frame("B", payloadB))
			if f := w.(http.Flusher); f != nil {
				f.Flush()
			}
			<-r.Context().Done()
		default:
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := tinyBackoff(New(srv.URL))
	events, _ := c.Stream(ctx)

	first := <-events
	if first.MetaID != "evt-A" {
		t.Fatalf("first event = %+v", first)
	}
	got := <-sawHeader
	if got != "A" {
		t.Fatalf("reconnect sent Last-Event-ID %q, want %q", got, "A")
	}
	second := <-events
	if second.MetaID != "evt-B" {
		t.Fatalf("second event = %+v", second)
	}
	if c.LastEventID() != "B" {
		t.Fatalf("LastEventID() = %q, want B", c.LastEventID())
	}
	cancel()
}

func TestSourceRetriesHTTPFailure(t *testing.T) { // R2: non-200 → backoff → retry
	var conns atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conns.Add(1) == 1 {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, frame("", payloadA))
		if f := w.(http.Flusher); f != nil {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := tinyBackoff(New(srv.URL))
	events, errc := c.Stream(ctx)

	ev := <-events
	if ev.MetaID != "evt-A" {
		t.Fatalf("event = %+v", ev)
	}
	// The 503 was reported before the successful reconnect — already buffered.
	select {
	case err := <-errc:
		if !strings.Contains(err.Error(), "503") {
			t.Fatalf("errc error = %v, want the 503 failure", err)
		}
	default:
		t.Fatal("errc has no failure record for the 503")
	}
	cancel()
	assertCloses(t, events, errc)
}

func TestSourceCancelClosesChannels(t *testing.T) { // R6 shape at the source
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, frame("", payloadA))
		if f := w.(http.Flusher); f != nil {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := tinyBackoff(New(srv.URL))
	events, errc := c.Stream(ctx)

	<-events // stream is alive
	cancel()
	assertCloses(t, events, errc)
}

// assertCloses fails unless both channels close within two seconds of the
// cancellation — the source must never leak its goroutine on shutdown.
func assertCloses[E any](t *testing.T, events <-chan E, errc <-chan error) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for events != nil || errc != nil {
		select {
		case _, ok := <-events:
			if !ok {
				events = nil
			}
		case _, ok := <-errc:
			if !ok {
				errc = nil
			}
		case <-deadline:
			t.Fatal("channels did not close after cancellation")
		}
	}
}
