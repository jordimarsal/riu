// Package source is the transport adapter that satisfies the
// wikievent.EventSource port over an SSE stream (Server-Sent Events).
//
// The protocol is line-based text: `event:`, `id:`, `data:` fields, `:`-prefix
// comments, blank line = end of event. Parsed with net/http + bufio.Scanner —
// no SSE library (design D4). Reconnects are bounded-exponential with jitter
// and resume via Last-Event-ID (requirement R2).
package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"math/rand/v2"

	"github.com/jordimarsal/riu/internal/wikievent"
)

// Backoff bounds for reconnects. Bounded exponential: base * 2^n capped at
// Max, with ±20% jitter so many clients do not sync their retries.
const (
	DefaultBackoffBase = 500 * time.Millisecond
	DefaultBackoffMax  = 30 * time.Second
)

// maxLineSize bounds one SSE line. The scanner default (64KiB) is too tight
// for fat edit payloads; 1MiB is generous and still bounded.
const maxLineSize = 1 << 20

// Client implements wikievent.EventSource over an SSE endpoint.
type Client struct {
	URL  string
	HTTP *http.Client

	base time.Duration // reconnect backoff base
	max  time.Duration // reconnect backoff cap

	mu      sync.Mutex   // guards lastID
	lastID  string       // last `id:` frame seen — sent back as Last-Event-ID
	invalid atomic.Int64 // malformed frames skipped: counted, never fatal (R10)
}

// New returns a Client for url with default backoff bounds.
func New(url string) *Client {
	return &Client{
		URL:  url,
		HTTP: &http.Client{}, // no timeout: streams are long-lived; ctx rules
		base: DefaultBackoffBase,
		max:  DefaultBackoffMax,
	}
}

// LastEventID returns the last event id seen on the wire (for observability).
func (c *Client) LastEventID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastID
}

// InvalidFrames reports how many frames were skipped as malformed.
func (c *Client) InvalidFrames() int64 { return c.invalid.Load() }

// Stream implements wikievent.EventSource. Both channels close when ctx is
// done (or on unrecoverable error). The consumer must drain both channels —
// it is the consumer contract that keeps this goroutine from leaking.
func (c *Client) Stream(ctx context.Context) (<-chan wikievent.Event, <-chan error) {
	events := make(chan wikievent.Event)
	errc := make(chan error, 8)
	go c.run(ctx, events, errc)
	return events, errc
}

// run owns the connect/read/reconnect loop until ctx is done.
func (c *Client) run(ctx context.Context, events chan<- wikievent.Event, errc chan<- error) {
	defer close(events)
	defer close(errc)

	b := backoff{base: c.base, max: c.max}

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
		if err != nil {
			// A bad URL will never fix itself: permanent, no retry.
			emit(ctx, errc, fmt.Errorf("source: request: %w", err))
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		if last := c.LastEventID(); last != "" {
			req.Header.Set("Last-Event-ID", last) // R2: resume where we left off
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return // cancellation masquerades as a transport error
			}
			emit(ctx, errc, fmt.Errorf("source: connect: %w", err))
			if !sleepCtx(ctx, b.next()) {
				return
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			emit(ctx, errc, fmt.Errorf("source: http %s", resp.Status))
			if !sleepCtx(ctx, b.next()) {
				return
			}
			continue
		}

		// A healthy connection resets the backoff: drops after a long lived
		// stream start from scratch, not from a grown exponent.
		b.reset()

		// Unblock the blocked read when ctx is cancelled: closing the body
		// makes the scanner fail, and the loop exits via ctx.Err() below.
		watchDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = resp.Body.Close()
			case <-watchDone:
			}
		}()

		readErr := c.readStream(ctx, resp.Body, events)
		close(watchDone)
		_ = resp.Body.Close()
		if ctx.Err() != nil {
			return
		}
		if readErr != nil {
			emit(ctx, errc, fmt.Errorf("source: stream: %w", readErr))
		}
		// Clean EOF or error: the stream dropped us — reconnect with resume.
		if !sleepCtx(ctx, b.next()) {
			return
		}
	}
}

// readStream parses SSE frames until EOF, an error, or a delivery that ctx
// cancels. Malformed payloads are counted (InvalidFrames) and skipped —
// one bad edit must never take down the river (R10).
func (c *Client) readStream(ctx context.Context, r io.ReadCloser, events chan<- wikievent.Event) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineSize) // ScanLines drops a trailing \r

	var (
		dataLines []string
		frameID   string
		hasData   bool
	)

	dispatch := func() bool { // false = ctx done: stop reading, run() exits cleanly
		defer func() { dataLines, frameID, hasData = nil, "", false }()
		if !hasData {
			return true // comment-only or empty frame: nothing to deliver
		}
		if frameID != "" {
			c.mu.Lock()
			c.lastID = frameID // single source of truth: reconnect header + observability
			c.mu.Unlock()
		}
		payload := []byte(strings.Join(dataLines, "\n")) // SSE: multi-line data joins with \n
		ev, err := wikievent.ParseEvent(payload)
		if err != nil {
			c.invalid.Add(1)
			return true
		}
		return emit(ctx, events, ev)
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, ":"):
			// comment / keepalive — ignore
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			hasData = true
		case strings.HasPrefix(line, "id:"):
			frameID = strings.TrimPrefix(strings.TrimPrefix(line, "id:"), " ")
			// SSE: an empty id value resets the last-event-id.
		case strings.HasPrefix(line, "event:"), strings.HasPrefix(line, "retry:"):
			// single event type on this feed; retry hints folded into our backoff
		case line == "":
			if !dispatch() {
				return nil
			}
		}
	}
	// A trailing frame without its blank line: deliver if the scanner ended
	// cleanly (EOF), otherwise the connection died mid-frame — count it.
	err := sc.Err()
	if err == nil {
		if !dispatch() {
			return nil
		}
		return nil
	}
	c.invalid.Add(1)
	return fmt.Errorf("reading stream: %w", err)
}

// emit sends v on ch, returning false when ctx is done first. Every blocking
// send in the source goes through here: cancellation always wins.
func emit[E any](ctx context.Context, ch chan<- E, v E) bool {
	select {
	case ch <- v:
		return true
	case <-ctx.Done():
		return false
	}
}

// sleepCtx waits d, returning false if ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// backoff is bounded exponential (base * 2^n, capped at max) with ±20% jitter.
type backoff struct {
	base time.Duration
	max  time.Duration
	n    int
}

func (b *backoff) next() time.Duration {
	d := b.base << min(b.n, 20) // shift caps at base*2^20 well above any sane max
	if d > b.max || d <= 0 {
		d = b.max
	}
	b.n++
	jitter := 0.8 + 0.4*rand.Float64() // [0.8, 1.2)
	return time.Duration(float64(d) * jitter)
}

func (b *backoff) reset() { b.n = 0 }
