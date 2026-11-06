package launchtest_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Story 18: tickets of one epic launched together share one cold package-manager
// store, so their installs run one at a time and the later launches wait in
// their deps step.
func TestColdStoreWave_SixTicketsOfOneEpicInstallOneAtATime(t *testing.T) {
	const (
		tickets = 6
		install = 300 * time.Millisecond
	)
	binDir, logPath := t.TempDir(), filepath.Join(t.TempDir(), "invocations.log")
	stub := fmt.Sprintf("#!/bin/sh\necho start >> %[1]s\nsleep %.1f\necho end >> %[1]s\n", logPath, install.Seconds())
	if err := os.WriteFile(filepath.Join(binDir, "pnpm"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	// The default install looks the manager up on the process PATH, so this test
	// cannot run in parallel.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	store, repo := t.TempDir(), testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "package.json", `{"name":"cold-store-wave"}`)
	testutil.WriteFile(t, repo, "pnpm-lock.yaml", "")
	testutil.CommitAll(t, repo, "add install files")
	for i := 1; i <= tickets; i++ {
		servertest.WriteTicket(t, store, "proj", "epic-a", fmt.Sprintf("%02d", i), "t", "")
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.MaxAgents, c.MaxAgentsPerRoot = tickets, tickets
	})
	t.Cleanup(func() {
		for i := 1; i <= tickets; i++ {
			fake.SetState(fmt.Sprintf("epic-a-iter-%02d", i), "done", "")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	began := time.Now()
	for i := 1; i <= tickets; i++ {
		addr := fmt.Sprintf("proj:epic-a/%02d", i)
		if res, err := h.Client.Queue.Add(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	if !servertest.Poll(50*time.Second, func() bool { return len(ofType(runLog(t, h), events.LaunchTimed)) == tickets }) {
		t.Fatalf("launch-timed = %d, want %d", len(ofType(runLog(t, h), events.LaunchTimed)), tickets)
	}
	wave := time.Since(began)

	if got, want := strings.Join(readLines(t, logPath), ","), strings.Repeat("start,end,", tickets-1)+"start,end"; got != want {
		t.Errorf("manager invocations = %s, want one install at a time: %s", got, want)
	}
	var deps []time.Duration
	for _, ev := range ofType(runLog(t, h), events.LaunchTimed) {
		deps = append(deps, ev.DepsStep)
	}
	slices.Sort(deps)
	// The k-th launch to install waits out the k-1 before it.
	for k, d := range deps {
		if want := time.Duration(k+1) * install; d < want {
			t.Errorf("deps step #%d = %v, want at least %v", k+1, d, want)
		}
	}
	if bound := tickets*install + 20*time.Second; wave > bound {
		t.Errorf("wave took %v, want under %v", wave, bound)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}
