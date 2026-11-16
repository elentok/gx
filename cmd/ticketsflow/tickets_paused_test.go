package ticketsflow

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/wire"
)

// pausedServer serves the three reads `tickets paused` makes on a unix socket.
func pausedServer(t *testing.T, detail wire.TicketDetail, history []runlog.Event, iterations []wire.IterationInfo) *apiclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	write := func(v any) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(v) }
	}
	mux.HandleFunc("/v1/tickets/detail", write(detail))
	mux.HandleFunc("/v1/tickets/history", write(wire.History{Address: detail.Address, Events: history}))
	mux.HandleFunc("/v1/iterations", write(iterations))

	// A unix socket path must stay short, so t.TempDir is too long on macOS.
	dir, err := os.MkdirTemp("", "gxp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return apiclient.New(sock)
}

func runPaused(t *testing.T, cl *apiclient.Client, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Stdout:    &out,
		NewClient: func() (*apiclient.Client, error) { return cl, nil },
	}}
	cmd := newTicketsPausedCmd(d)
	root := &cobra.Command{Use: "root"}
	root.AddCommand(cmd)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"paused"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestTicketsPaused_SummarisesPauseEventsWorktreeAndPane(t *testing.T) {
	t.Parallel()
	cl := pausedServer(t,
		wire.TicketDetail{
			Address: "proj:epic/03", Title: "Do the thing", Status: "paused",
			PauseReason: "self-reported", PauseLabel: "Self reported", PauseText: "which base?",
		},
		[]runlog.Event{
			{Type: string(events.IterationStarted), Ticket: "03"},
			{Type: string(events.SchedulerScan)},
			{Type: string(events.Paused), Ticket: "03", Reason: "self-reported"},
		},
		[]wire.IterationInfo{
			{Address: "proj:epic/03", Pane: "pane-7", Worktree: "/wt/epic-03", Branch: "ralph-loop/epic"},
			{Address: "proj:epic/04", Pane: "pane-9", Worktree: "/wt/other"},
		},
	)

	out, err := runPaused(t, cl, "proj:epic/03")
	if err != nil {
		t.Fatalf("tickets paused: %v\n%s", err, out)
	}

	for _, want := range []string{
		"Self reported", "self-reported", "which base?",
		"iteration-started", "paused",
		"/wt/epic-03", "ralph-loop/epic", "pane-7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "scheduler-scan") || strings.Contains(out, "pane-9") {
		t.Errorf("output has a scan event or another ticket's pane:\n%s", out)
	}
	if n := strings.Count(out, "\n"); n >= 30 {
		t.Errorf("output has %d lines, want under 30", n)
	}
}

func TestTicketsPaused_NoLiveIteration(t *testing.T) {
	t.Parallel()
	cl := pausedServer(t,
		wire.TicketDetail{Address: "proj:epic/03", Title: "T", Status: "paused", PauseReason: "retry", PauseLabel: "Retry"},
		nil, nil)

	out, err := runPaused(t, cl, "proj:epic/03")
	if err != nil {
		t.Fatalf("tickets paused: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pane: none") || !strings.Contains(out, "worktree: none") {
		t.Errorf("output = %s, want no pane and no worktree", out)
	}
}

func TestTicketsPaused_NotPausedIsError(t *testing.T) {
	t.Parallel()
	cl := pausedServer(t, wire.TicketDetail{Address: "proj:epic/03", Status: "open"}, nil, nil)

	if out, err := runPaused(t, cl, "proj:epic/03"); err == nil {
		t.Fatalf("want an error for a ticket that is not paused, got output:\n%s", out)
	}
}
