// Package store persists which Discord thread belongs to which Devin session.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Link struct {
	ThreadID     string    `json:"thread_id"`
	SessionID    string    `json:"session_id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	OwnerID      string    `json:"owner_id"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
	StatusDetail string    `json:"status_detail"`
	// Relayed holds the event IDs of Devin messages already posted to the thread.
	Relayed []string `json:"relayed"`
	// PRs holds pull request URLs already announced in the thread.
	PRs []string `json:"prs"`
	// Active is false once the session ended or went idle; a thread message reactivates it.
	Active bool `json:"active"`
}

func (l Link) HasRelayed(eventID string) bool {
	return contains(l.Relayed, eventID)
}

func (l Link) HasPR(url string) bool {
	return contains(l.PRs, url)
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]Link
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]Link{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s.links); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Get(threadID string) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[threadID]
	return l, ok
}

func (s *Store) Put(l Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[l.ThreadID] = l
	return s.save()
}

// Update applies fn to the stored link, if it exists, and persists the result.
func (s *Store) Update(threadID string, fn func(*Link)) (Link, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[threadID]
	if !ok {
		return Link{}, false, nil
	}
	fn(&l)
	s.links[threadID] = l
	return l, true, s.save()
}

func (s *Store) Active() []Link {
	return s.filter(func(l Link) bool { return l.Active })
}

func (s *Store) ByOwner(ownerID string) []Link {
	return s.filter(func(l Link) bool { return l.OwnerID == ownerID })
}

func (s *Store) filter(keep func(Link) bool) []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Link
	for _, l := range s.links {
		if keep(l) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.links, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".devincord-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
