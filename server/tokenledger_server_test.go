package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tokenledger"
)

func TestTokenLedgerServerKeptAcrossRestart(t *testing.T) {
	// A short path: the unix socket path has a length limit.
	base, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	stateDir := filepath.Join(base, "s")

	start := func() *Server {
		s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, Build: "b"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	stop := func(s *Server) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- s.Serve(ctx) }()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	s := start()
	k := tokenledger.Key{Day: "2026-01-01", Project: "gx", Group: tokenledger.GroupQueue, Harness: "claude"}
	if err := s.tokens.Add(tokenledger.Delta{Key: k, Tokens: 9, Dollars: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.tokens.SetSeen("s1", tokenledger.Seen{Tokens: 9}); err != nil {
		t.Fatal(err)
	}
	stop(s)

	if _, err := os.Stat(filepath.Join(stateDir, tokenledger.FileName)); err != nil {
		t.Fatalf("ledger not in the state dir: %v", err)
	}
	s = start()
	rows, err := s.tokens.Day("2026-01-01")
	if err != nil || len(rows) != 1 || rows[0].Tokens != 9 {
		t.Fatalf("rows after restart = %+v, err %v", rows, err)
	}
	if seen, ok, _ := s.tokens.Seen("s1"); !ok || seen.Tokens != 9 {
		t.Fatalf("seen after restart = %+v %v", seen, ok)
	}
	stop(s)
}
