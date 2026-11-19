package wikievent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const validEvent = `{"$schema":"/mediawiki/recentchange/1.0.0",` +
	`"meta":{"domain":"ca.wikipedia.org","id":"evt-abc-123","dt":"2026-11-19T12:25:50Z"},` +
	`"id":42,"type":"edit","title":"Riu","user":"jordi","bot":false,` +
	`"comment":"primer test","length":{"old":10,"new":12}}`

var wantValid = Event{
	Domain:  "ca.wikipedia.org",
	MetaID:  "evt-abc-123",
	WikiID:  42,
	Type:    "edit",
	Title:   "Riu",
	User:    "jordi",
	Bot:     false,
	Comment: "primer test",
	OldLen:  10,
	NewLen:  12,
	At:      time.Date(2026, 11, 19, 12, 25, 50, 0, time.UTC),
}

func TestParseEvent(t *testing.T) {
	noLength := strings.Replace(validEvent, `"length":{"old":10,"new":12}`, `"length":{}`, 1)

	tests := []struct {
		name    string
		payload string
		want    Event
		wantErr error
	}{
		{"valid edit", validEvent, wantValid, nil},
		{"length absent → zero lens", noLength,
			mustZeroLens(wantValid), nil},
		{"truncated payload", validEvent[:20], Event{}, ErrMalformed},
		{"empty payload", ``, Event{}, ErrMalformed},
		{"unknown schema", strings.Replace(validEvent, "recentchange", "otherstream", 1),
			Event{}, ErrUnknownSchema},
		{"wrong type for id (string)", strings.Replace(validEvent, `"id":42`, `"id":"42"`, 1),
			Event{}, ErrMalformed},
		{"missing meta.id", strings.Replace(validEvent, `"id":"evt-abc-123",`, ``, 1),
			Event{}, ErrMalformed},
		{"empty meta.domain", strings.Replace(validEvent, `ca.wikipedia.org`, ``, 1),
			Event{}, ErrMalformed},
		{"unparsable meta.dt", strings.Replace(validEvent, `2026-11-19T12:25:50Z`, `yesterday`, 1),
			Event{}, ErrMalformed},
		{"negative id", strings.Replace(validEvent, `"id":42`, `"id":-1`, 1),
			Event{}, ErrMalformed},
		{"empty type", strings.Replace(validEvent, `"type":"edit"`, `"type":""`, 1),
			Event{}, ErrMalformed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEvent([]byte(tt.payload))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseEvent() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEvent() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseEvent() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func mustZeroLens(e Event) Event {
	e.OldLen, e.NewLen = 0, 0
	return e
}

func TestParseEventErrorsWrapSentinels(t *testing.T) {
	_, err := ParseEvent([]byte(`{`))
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("truncated payload error = %v, want wrap of ErrMalformed", err)
	}
	if !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("error should keep the ErrMalformed context, got: %v", err)
	}
}

func TestIdentity(t *testing.T) {
	domain, id := wantValid.Identity()
	if domain != "ca.wikipedia.org" || id != "evt-abc-123" {
		t.Fatalf("Identity() = (%q, %q)", domain, id)
	}
}
