// Package relay mirrors Devin session activity into Discord threads.
package relay

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/atluixx/devincord/internal/devin"
	"github.com/atluixx/devincord/internal/store"
)

const (
	discordLimit = 2000
	maxRelayed   = 500
)

type API interface {
	GetSession(ctx context.Context, sessionID string) (devin.Session, error)
	ListMessages(ctx context.Context, sessionID string) ([]devin.Message, error)
}

// Poster sends plain text to a Discord thread.
type Poster interface {
	Post(threadID, content string) error
}

type Relay struct {
	api    API
	store  *store.Store
	poster Poster
}

func New(api API, st *store.Store, poster Poster) *Relay {
	return &Relay{api: api, store: st, poster: poster}
}

func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		r.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Relay) Tick(ctx context.Context) {
	for _, link := range r.store.Active() {
		if ctx.Err() != nil {
			return
		}
		if err := r.Sync(ctx, link.ThreadID); err != nil {
			slog.Warn("relay sync failed", slog.String("session", link.SessionID), slog.Any("err", err))
		}
	}
}

// Sync posts new Devin messages, PRs and status changes for one thread.
func (r *Relay) Sync(ctx context.Context, threadID string) error {
	link, ok := r.store.Get(threadID)
	if !ok {
		return nil
	}
	msgs, err := r.api.ListMessages(ctx, link.SessionID)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Source != "devin" || link.HasRelayed(m.EventID) || strings.TrimSpace(m.Message) == "" {
			continue
		}
		if err := r.post(threadID, m.Message); err != nil {
			return err
		}
		link, _, err = r.store.Update(threadID, func(l *store.Link) {
			l.Relayed = append(l.Relayed, m.EventID)
			if len(l.Relayed) > maxRelayed {
				l.Relayed = l.Relayed[len(l.Relayed)-maxRelayed:]
			}
		})
		if err != nil {
			return err
		}
	}

	s, err := r.api.GetSession(ctx, link.SessionID)
	if err != nil {
		return err
	}
	for _, pr := range s.PullRequests {
		if pr.URL == "" || link.HasPR(pr.URL) {
			continue
		}
		if err := r.post(threadID, "Pull request: "+pr.URL); err != nil {
			return err
		}
		if link, _, err = r.store.Update(threadID, func(l *store.Link) { l.PRs = append(l.PRs, pr.URL) }); err != nil {
			return err
		}
	}

	if s.Status == link.Status && s.StatusDetail == link.StatusDetail {
		return nil
	}
	if line := StatusLine(s); line != "" && notable(s) {
		if err := r.post(threadID, line); err != nil {
			return err
		}
	}
	_, _, err = r.store.Update(threadID, func(l *store.Link) {
		l.Status = s.Status
		l.StatusDetail = s.StatusDetail
		if s.Title != "" {
			l.Title = s.Title
		}
		l.Active = !s.Ended() && s.Status != "suspended"
	})
	return err
}

func (r *Relay) post(threadID, content string) error {
	for _, chunk := range Chunk(content, discordLimit) {
		if err := r.poster.Post(threadID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func notable(s devin.Session) bool {
	switch s.Status {
	case "exit", "error", "suspended":
		return true
	case "running":
		return s.StatusDetail != "" && s.StatusDetail != "working"
	}
	return false
}

// StatusLine describes a session state for humans, or "" if there is nothing to say.
func StatusLine(s devin.Session) string {
	switch s.Status {
	case "new", "claimed", "resuming":
		return "Devin is starting up."
	case "running":
		switch s.StatusDetail {
		case "working", "":
			return "Devin is working."
		case "waiting_for_user":
			return "Devin is waiting for your reply. Answer in this thread."
		case "waiting_for_approval":
			return "Devin is waiting for approval: " + s.URL
		case "finished":
			return "Devin finished. Reply here to follow up."
		}
	case "suspended":
		if s.StatusDetail != "" && s.StatusDetail != "inactivity" && s.StatusDetail != "user_request" {
			return fmt.Sprintf("Session suspended (%s). Reply here to resume.", humanize(s.StatusDetail))
		}
		return "Session went to sleep. Reply here to wake it up."
	case "exit":
		return "Session ended."
	case "error":
		return "Session hit an error: " + s.URL
	}
	return ""
}

func humanize(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

// Chunk splits text into pieces of at most limit bytes, preferring line breaks.
func Chunk(text string, limit int) []string {
	var out []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit], "\n")
		if cut <= 0 {
			cut = limit
			for cut > 0 && !utf8Start(text[cut]) {
				cut--
			}
		}
		out = append(out, text[:cut])
		text = strings.TrimPrefix(text[cut:], "\n")
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}

func utf8Start(b byte) bool {
	return b&0xC0 != 0x80
}
