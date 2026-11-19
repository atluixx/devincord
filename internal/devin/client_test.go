package devin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/organizations/org-1/sessions" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer cog_x" {
			t.Errorf("auth header %q", got)
		}
		var body CreateSessionRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Prompt != "fix it" || body.MaxACULimit != 3 || len(body.Tags) != 1 {
			t.Errorf("body %+v", body)
		}
		_, _ = w.Write([]byte(`{"session_id":"devin-1","url":"https://app.devin.ai/sessions/1","status":"new"}`))
	}))
	defer srv.Close()

	s, err := New(srv.URL+"/", "org-1", "cog_x").CreateSession(context.Background(),
		CreateSessionRequest{Prompt: "fix it", Tags: []string{"discord"}, MaxACULimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if s.SessionID != "devin-1" || s.Status != "new" || s.Ended() {
		t.Fatalf("session %+v", s)
	}
}

func TestListMessagesPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/organizations/org-1/sessions/devin-1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		switch r.URL.Query().Get("after") {
		case "":
			_, _ = w.Write([]byte(`{"items":[{"event_id":"a","source":"user","message":"hi","created_at":1}],"end_cursor":"c1","has_next_page":true}`))
		case "c1":
			_, _ = w.Write([]byte(`{"items":[{"event_id":"b","source":"devin","message":"hello","created_at":2}],"end_cursor":null,"has_next_page":false}`))
		default:
			t.Errorf("cursor %q", r.URL.Query().Get("after"))
		}
	}))
	defer srv.Close()

	msgs, err := New(srv.URL, "org-1", "k").ListMessages(context.Background(), "devin-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[1].EventID != "b" || msgs[1].Source != "devin" {
		t.Fatalf("messages %+v", msgs)
	}
}

func TestErrorsIncludeDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"title":"Forbidden","detail":"missing UseDevinSessions"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "org-1", "k").SendMessage(context.Background(), "devin-1", "hi")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 || apiErr.Error() != "devin api: 403: missing UseDevinSessions" {
		t.Fatalf("err %v", err)
	}
}

func TestEnded(t *testing.T) {
	for status, want := range map[string]bool{"running": false, "suspended": false, "exit": true, "error": true} {
		if got := (Session{Status: status}).Ended(); got != want {
			t.Errorf("%s: got %v", status, got)
		}
	}
}
