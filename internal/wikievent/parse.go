package wikievent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// schemaPrefix is the accepted family of wire schemas. Unknown schemas are
// rejected loudly (ErrUnknownSchema): silent drift is how pipelines lie.
const schemaPrefix = "/mediawiki/recentchange/"

// raw mirrors the recentchange wire shape. Unknown wire fields are tolerated
// on purpose — the schema grows over time; only $schema is checked. Pointer
// fields model "may be absent" without zero-value lies.
type raw struct {
	Schema string `json:"$schema"`
	Meta   struct {
		Domain string `json:"domain"`
		ID     string `json:"id"`
		DT     string `json:"dt"`
	} `json:"meta"`
	ID      int64  `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	User    string `json:"user"`
	Bot     bool   `json:"bot"`
	Comment string `json:"comment"`
	Length  struct {
		Old *int64 `json:"old"`
		New *int64 `json:"new"`
	} `json:"length"`
}

// ParseEvent normalizes one raw wire payload into an Event. It is pure: bytes
// in, value or error out. Errors wrap ErrMalformed (or ErrUnknownSchema) so
// callers classify with errors.Is.
func ParseEvent(b []byte) (Event, error) {
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		return Event{}, fmt.Errorf("%w: %s", ErrMalformed, err)
	}
	if !strings.HasPrefix(r.Schema, schemaPrefix) {
		return Event{}, fmt.Errorf("%w: %q", ErrUnknownSchema, r.Schema)
	}
	at, err := time.Parse(time.RFC3339, r.Meta.DT)
	if err != nil {
		return Event{}, fmt.Errorf("%w: meta.dt %q: %s", ErrMalformed, r.Meta.DT, err)
	}
	if r.Meta.Domain == "" {
		return Event{}, fmt.Errorf("%w: empty meta.domain", ErrMalformed)
	}
	if r.Meta.ID == "" {
		return Event{}, fmt.Errorf("%w: empty meta.id", ErrMalformed)
	}
	if r.ID < 0 {
		return Event{}, fmt.Errorf("%w: negative id %d", ErrMalformed, r.ID)
	}
	if r.Type == "" {
		return Event{}, fmt.Errorf("%w: empty type", ErrMalformed)
	}

	e := Event{
		Domain:  r.Meta.Domain,
		MetaID:  r.Meta.ID,
		WikiID:  r.ID,
		Type:    r.Type,
		Title:   r.Title,
		User:    r.User,
		Bot:     r.Bot,
		Comment: r.Comment,
		At:      at.UTC(),
	}
	if r.Length.Old != nil {
		e.OldLen = *r.Length.Old
	}
	if r.Length.New != nil {
		e.NewLen = *r.Length.New
	}
	return e, nil
}
