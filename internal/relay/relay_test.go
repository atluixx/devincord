package relay

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/atluixx/devincord/internal/devin"
	"github.com/atluixx/devincord/internal/store"
)

type fakeAPI struct {
	session devin.Session
	msgs    []devin.Message
}

func (f *fakeAPI) GetSession(context.Context, string) (devin.Session, error) { return f.session, nil }
func (f *fakeAPI) ListMessages(context.Context, string) ([]devin.Message, error) {
	return f.msgs, nil
}

type fakePoster struct{ posts []string }

func (f *fakePoster) Post(_ string, content string) error {
	f.posts = append(f.posts, content)
	return nil
}

func setup(t *testing.T) (*fakeAPI, *fakePoster, *store.Store, *Relay) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(store.Link{ThreadID: "t", SessionID: "devin-1", Active: true}); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{session: devin.Session{SessionID: "devin-1", URL: "https://app.devin.ai/sessions/1", Status: "running", StatusDetail: "working"}}
	poster := &fakePoster{}
	return api, poster, st, New(api, st, poster)
}

func TestSyncRelaysDevinMessagesOnce(t *testing.T) {
	api, poster, _, r := setup(t)
	api.msgs = []devin.Message{
		{EventID: "1", Source: "user", Message: "do it"},
		{EventID: "2", Source: "devin", Message: "on it"},
	}
	ctx := context.Background()
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	api.msgs = append(api.msgs, devin.Message{EventID: "3", Source: "devin", Message: "done"})
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(poster.posts, "|") != "on it|done" {
		t.Fatalf("posts %q", poster.posts)
	}
}

func TestSyncAnnouncesStatusAndPRs(t *testing.T) {
	api, poster, st, r := setup(t)
	ctx := context.Background()
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if len(poster.posts) != 0 {
		t.Fatalf("working status should be silent, got %q", poster.posts)
	}

	api.session.StatusDetail = "waiting_for_user"
	api.session.PullRequests = []devin.PullRequest{{URL: "https://github.com/o/r/pull/1"}}
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	want := "Pull request: https://github.com/o/r/pull/1|Devin is waiting for your reply. Answer in this thread."
	if got := strings.Join(poster.posts, "|"); got != want {
		t.Fatalf("posts %q", got)
	}

	api.session.Status, api.session.StatusDetail = "suspended", "inactivity"
	if err := r.Sync(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.Get("t"); l.Active {
		t.Fatal("suspended session should stop polling")
	}
	if got := poster.posts[len(poster.posts)-1]; got != "Session went to sleep. Reply here to wake it up." {
		t.Fatalf("last post %q", got)
	}
}

func TestChunk(t *testing.T) {
	text := strings.Repeat("a", 1500) + "\n" + strings.Repeat("b", 1000)
	got := Chunk(text, 2000)
	if len(got) != 2 || len(got[0]) != 1500 || len(got[1]) != 1000 {
		t.Fatalf("chunks %v", lens(got))
	}
	long := strings.Repeat("é", 1500)
	for _, c := range Chunk(long, 2000) {
		if len(c) > 2000 || !utf8.ValidString(c) {
			t.Fatalf("bad chunk len %d valid %v", len(c), utf8.ValidString(c))
		}
	}
	if Chunk("", 10) != nil {
		t.Fatal("empty text should produce no chunks")
	}
}

func lens(s []string) []int {
	var out []int
	for _, c := range s {
		out = append(out, len(c))
	}
	return out
}
