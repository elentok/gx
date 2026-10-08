package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// fakeSource serves one ticket whose status advances one step per snapshot.
// failFirst snapshots fail first, standing in for a server restart.
type fakeSource struct {
	file      string
	statuses  []string
	failFirst int
	snaps     int
}

func (f *fakeSource) Snapshot(context.Context) (server.Snapshot, error) {
	f.snaps++
	if f.snaps <= f.failFirst {
		return server.Snapshot{}, errors.New("connection refused")
	}
	i := min(f.snaps-f.failFirst-1, len(f.statuses)-1)
	return server.Snapshot{Seq: uint64(f.snaps), Tickets: []server.TicketInfo{
		{Address: "gx:p/1", Status: f.statuses[i], File: f.file},
	}}, nil
}

// Events delivers one event for the ticket, then ends like a dropped stream.
func (f *fakeSource) Events(context.Context, uint64) (<-chan server.Event, error) {
	ch := make(chan server.Event, 1)
	ch <- server.Event{Address: "gx:p/1"}
	close(ch)
	return ch, nil
}

func runWait(t *testing.T, f *fakeSource) (stdout string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := runOneOffWait(ctx, f, &out, &errOut, "gx:p/1")
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return out.String(), exitErr.Code
	}
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	return out.String(), 0
}

func TestOneOffWait_DonePrintsResultAndExitsZero(t *testing.T) {
	file := filepath.Join(t.TempDir(), "1.md")
	body := "---\nid: 1\ntitle: t\nstatus: done\ntype: prompt\n---\n\n## Result\n\nall good\n\n## Comments\n\nnoise\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runWait(t, &fakeSource{file: file, statuses: []string{"claimed", "done"}})
	if code != 0 || strings.TrimSpace(out) != "all good" {
		t.Errorf("code = %d, out = %q; want 0 and the Result only", code, out)
	}
}

func TestOneOffWait_ExitCodesByStatus(t *testing.T) {
	for status, want := range map[string]int{"needs-answer": 3, "needs-repair": 4, "cancelled": 5} {
		if _, code := runWait(t, &fakeSource{statuses: []string{"open", status}}); code != want {
			t.Errorf("%s: code = %d; want %d", status, code, want)
		}
	}
}

func TestOneOffWait_SurvivesServerRestart(t *testing.T) {
	old := waitRetry
	t.Cleanup(func() { waitRetry = old })
	waitRetry = time.Millisecond
	f := &fakeSource{statuses: []string{"claimed", "needs-repair"}, failFirst: 3}
	if _, code := runWait(t, f); code != 4 {
		t.Errorf("code = %d; want 4 after the outage", code)
	}
}
