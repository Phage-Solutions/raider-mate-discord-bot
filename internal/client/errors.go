package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// APIError is a 4xx or 5xx from the service, carrying the message it sent.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return "service returned " + http.StatusText(e.Status) + ": " + e.Message
}

// Safe reports whether Message can be shown to a player. The service writes its 4xx
// messages for a human ("not your character", "raid lead required"); its 5xx messages
// are the word "internal error" over whatever actually broke, which belongs in the log
// and not in Discord.
func (e *APIError) Safe() bool {
	return e.Status < http.StatusInternalServerError
}

// ErrCompIsManual means the comp is hand-built and the assigner refused to touch it.
// The raid lead's board survives; the bot says so in a sentence rather than showing an
// error.
var ErrCompIsManual = errors.New("comp is manual")

// ErrAlreadyDecided means another raid lead approved or rejected the late request
// first.
var ErrAlreadyDecided = errors.New("late request already decided")

// ErrNotFound covers both "no such thing" and "not in your guild": the service does not
// distinguish them, on purpose, so neither does this.
var ErrNotFound = errors.New("not found")

// ErrForbidden means the action needs a capability the caller does not have.
var ErrForbidden = errors.New("forbidden")

// readAPIError turns an error response into an APIError, joined with a sentinel where
// the status has one meaning across the whole API. Statuses that mean different things
// per endpoint (409 in particular) stay unjoined and are mapped by the caller.
func readAPIError(resp *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	// A body that will not decode is not worth reporting over the status itself: the
	// service always sends {"error": ...}, so this is a proxy or a panic mid-write.
	if raw, err := io.ReadAll(resp.Body); err == nil {
		_ = json.Unmarshal(raw, &body)
	}

	apiErr := &APIError{Status: resp.StatusCode, Message: body.Error}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return errors.Join(ErrNotFound, apiErr)
	case http.StatusForbidden:
		return errors.Join(ErrForbidden, apiErr)
	default:
		return apiErr
	}
}
