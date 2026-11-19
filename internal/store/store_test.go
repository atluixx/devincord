package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPersistsAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.Put(Link{ThreadID: "t1", SessionID: "devin-1", OwnerID: "u1", Active: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Link{ThreadID: "t2", SessionID: "devin-2", OwnerID: "u1", CreatedAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Update("t1", func(l *Link) { l.Relayed = append(l.Relayed, "e1") }); !ok || err != nil {
		t.Fatalf("update: %v %v", ok, err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := reopened.Get("t1")
	if !ok || !l.HasRelayed("e1") || l.HasRelayed("e2") {
		t.Fatalf("link %+v", l)
	}
	if got := reopened.Active(); len(got) != 1 || got[0].ThreadID != "t1" {
		t.Fatalf("active %+v", got)
	}
	if got := reopened.ByOwner("u1"); len(got) != 2 || got[0].ThreadID != "t2" {
		t.Fatalf("by owner should be newest first: %+v", got)
	}
	if _, ok, _ := reopened.Update("missing", func(*Link) {}); ok {
		t.Fatal("update of missing link reported ok")
	}
}
