package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "secret-key"

func testActor() Actor {
	return Actor{DiscordID: 1234, GuildID: 5678, RoleIDs: []uint64{781, 799}, GuildAdmin: true}
}

// newTestClient stands up a fake service and points a Client at it. The handler sees
// every request, so header assertions live inside it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(server.URL, testKey)
}

func TestRequestsCarryTheKeyAndTheActor(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		writeJSON(t, w, http.StatusOK, []Character{})
	})

	if _, err := c.GuildCharacters(context.Background(), testActor()); err != nil {
		t.Fatalf("listing characters: %v", err)
	}

	for header, want := range map[string]string{
		"Authorization":       "Bearer " + testKey,
		"X-Actor-Discord-Id":  "1234",
		"X-Actor-Guild-Id":    "5678",
		"X-Actor-Roles":       "781,799",
		"X-Actor-Guild-Admin": "true",
	} {
		if got.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, got.Get(header), want)
		}
	}
}

// Snowflakes exceed 2^53 and do not survive JSON's float64. They stay decimal strings
// on the wire, headers included, and this is the cheapest place to notice if that ever
// stops being true.
func TestSnowflakesAreSentAsDecimalStrings(t *testing.T) {
	var got string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Actor-Discord-Id")
		writeJSON(t, w, http.StatusOK, []Character{})
	})

	actor := testActor()
	actor.DiscordID = 9007199254740993 // 2^53 + 1
	if _, err := c.GuildCharacters(context.Background(), actor); err != nil {
		t.Fatalf("listing characters: %v", err)
	}

	if got != "9007199254740993" {
		t.Errorf("discord id header = %q, want the exact value", got)
	}
}

// The outbox is the bot talking as itself. Sending actor headers there would be
// harmless today and misleading tomorrow, since the route takes no actor at all.
func TestClaimingNotificationsSendsNoActorHeaders(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		writeJSON(t, w, http.StatusOK, []Notification{})
	})

	if _, err := c.ClaimNotifications(context.Background(), 50); err != nil {
		t.Fatalf("claiming notifications: %v", err)
	}

	for header := range got {
		if strings.HasPrefix(header, "X-Actor-") {
			t.Errorf("request carried %s, want no actor headers on the outbox route", header)
		}
	}
}

func TestLinksAreParsedFromTheResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id": "e1", "name": "main", "mode": "AUTO", "slots": []any{},
			"_links": map[string]any{
				"self": map[string]string{"href": "/api/events/e1/comps/main"},
				"lock": map[string]string{"href": "/api/events/e1/comps/main/lock", "method": "POST"},
			},
		})
	})

	board, err := c.Comp(context.Background(), testActor(), "e1", DefaultComp)
	if err != nil {
		t.Fatalf("reading comp: %v", err)
	}

	if !board.Links.Has("lock") {
		t.Error("links has no lock, want the transition the service offered")
	}
	if board.Links.Has("save") {
		t.Error("links has save, want the absence of a link to be the answer")
	}
	if link, _ := board.Links.Get("lock"); link.Method != http.MethodPost {
		t.Errorf("lock method = %q, want POST", link.Method)
	}
}

// Past the deadline a player's signup is not refused, it becomes a request for the
// raid lead. A client that flattened 202 into "written" would tell them they are in.
func TestASignupPastTheDeadlineComesBackAsALateRequest(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusAccepted, LateRequest{
			ID: "r1", CharacterID: "c1", Status: StatusConfirmed, State: RequestPending,
		})
	})

	result, err := c.Signup(context.Background(), testActor(), "e1", "c1",
		WriteSignup{Status: StatusConfirmed})
	if err != nil {
		t.Fatalf("writing signup: %v", err)
	}

	if !result.Filed() {
		t.Fatal("result is a signup, want a filed late request")
	}
	if result.Signup != nil {
		t.Error("result carries a signup as well as a request, want only the request")
	}
	if result.LateRequest.State != RequestPending {
		t.Errorf("state = %q, want PENDING", result.LateRequest.State)
	}
}

func TestASignupInsideTheDeadlineComesBackAsASignup(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, Signup{ID: "s1", CharacterID: "c1", Status: StatusConfirmed})
	})

	result, err := c.Signup(context.Background(), testActor(), "e1", "c1",
		WriteSignup{Status: StatusConfirmed})
	if err != nil {
		t.Fatalf("writing signup: %v", err)
	}

	if result.Filed() {
		t.Fatal("result is a late request, want a written signup")
	}
	if result.Signup.Status != StatusConfirmed {
		t.Errorf("status = %q, want CONFIRMED", result.Signup.Status)
	}
}

// The service says which statuses this caller may write. Dropping the field on the
// floor would leave a caller with a per-actor surface guessing, and guessing at what
// the API allows is what hard rule 5 forbids.
func TestASignupCarriesTheStatusesTheCallerMayWrite(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, Signup{
			ID: "s1", CharacterID: "c1", Status: StatusAbsent,
			AllowedStatuses: []SignupStatus{
				StatusConfirmed, StatusTentative, StatusDeclined, StatusLate, StatusAbsent,
			},
		})
	})

	result, err := c.Signup(context.Background(), testActor(), "e1", "c1",
		WriteSignup{Status: StatusAbsent})
	if err != nil {
		t.Fatalf("writing signup: %v", err)
	}

	got := result.Signup.AllowedStatuses
	if len(got) != 5 {
		t.Fatalf("allowed statuses = %v, want the five a player may write", got)
	}
	for _, status := range got {
		if status == StatusNoShow {
			t.Errorf("allowed statuses = %v, want NO_SHOW withheld from a player", got)
		}
	}
}

func TestLockingAManualCompIsNotAGenericFailure(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusConflict, map[string]string{"error": "comp is manual; convert it before locking"})
	})

	_, err := c.LockComp(context.Background(), testActor(), "e1", DefaultComp)
	if !errors.Is(err, ErrCompIsManual) {
		t.Errorf("error = %v, want ErrCompIsManual", err)
	}
}

func TestA404IsReportedAsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]string{"error": "character not found"})
	})

	_, err := c.Comp(context.Background(), testActor(), "e1", DefaultComp)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// A 4xx message is written for a human and is worth passing on. A 5xx message is
// "internal error" over whatever actually broke, and belongs in the log.
func TestOnlyClientErrorMessagesAreSafeToShow(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusForbidden, map[string]string{"error": "not your character"})
	})

	err := c.SetCharacterRoles(context.Background(), testActor(), "c1",
		[]RoleChoice{{Role: RoleTank, Priority: 1}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an APIError", err)
	}
	if !apiErr.Safe() {
		t.Error("Safe() = false on a 403, want the service's own wording shown")
	}
	if apiErr.Message != "not your character" {
		t.Errorf("message = %q, want the service's wording preserved", apiErr.Message)
	}

	server := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	})
	err = server.SetCharacterRoles(context.Background(), testActor(), "c1", nil)
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an APIError", err)
	}
	if apiErr.Safe() {
		t.Error("Safe() = true on a 500, want it withheld from Discord")
	}
}

func TestFollowUsesTheLinksHrefAndMethod(t *testing.T) {
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	link := Link{Href: "/api/notifications/n1/delivered", Method: http.MethodPost}
	if err := c.Follow(context.Background(), nil, link, nil, nil); err != nil {
		t.Fatalf("following link: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != link.Href {
		t.Errorf("request = %s %s, want POST %s", gotMethod, gotPath, link.Href)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("encoding test response: %v", err)
	}
}
