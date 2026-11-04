package limits_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
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

	start := func() *server.Server {
		s, err := server.New(server.Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, Build: "b"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	stop := func(s *server.Server) {
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
	if err := s.Limits().AddLedgerTokens(tokenledger.Delta{Key: k, Tokens: 9, Dollars: 1}); err != nil {
		t.Fatal(err)
	}
	stop(s)

	if _, err := os.Stat(filepath.Join(stateDir, tokenledger.FileName)); err != nil {
		t.Fatalf("ledger not in the state dir: %v", err)
	}
	s = start()
	rows, err := tokenledger.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rows.Day("2026-01-01")
	if err != nil || len(got) != 1 || got[0].Tokens != 9 {
		t.Fatalf("rows after restart = %+v, err %v", got, err)
	}
	stop(s)
}
