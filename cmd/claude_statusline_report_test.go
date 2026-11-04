package cmd

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/wire"
)

// syncBuilder lets a server handler look at the output while the command runs.
type syncBuilder struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *syncBuilder) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *syncBuilder) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

// reportServer is a unix-socket HTTP server that records /v1/usage/report calls.
type reportServer struct {
	mu       sync.Mutex
	reports  []wire.LimitsReport
	lineSeen []bool
	out      *syncBuilder
}

func startReportServer(t *testing.T, out *syncBuilder, handle func(w http.ResponseWriter, r *http.Request, rs *reportServer)) (*reportServer, *apiclient.Client) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gxrl")
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	rs := &reportServer{out: out}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rep wire.LimitsReport
		_ = json.NewDecoder(r.Body).Decode(&rep)
		rs.mu.Lock()
		rs.reports = append(rs.reports, rep)
		rs.lineSeen = append(rs.lineSeen, out.String() != "")
		rs.mu.Unlock()
		if handle == nil {
			_ = json.NewEncoder(w).Encode(wire.LimitsReportResult{})
			return
		}
		handle(w, r, rs)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = os.RemoveAll(dir)
	})
	return rs, apiclient.New(sock)
}

func (rs *reportServer) count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.reports)
}

func runStatuslineWith(t *testing.T, out *syncBuilder, dial func() (*apiclient.Client, error), payload string, args ...string) error {
	t.Helper()
	d := deps{
		stdin:        strings.NewReader(payload),
		stdout:       out,
		getwd:        func() (string, error) { return "/nonexistent", nil },
		serverClient: dial,
	}
	cmd := newClaudeStatuslineCmd(d)
	cmd.SetArgs(args)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	return cmd.Execute()
}

func noServer() (*apiclient.Client, error) { return nil, errors.New("not running") }

func baseline(t *testing.T, payload string, args ...string) string {
	t.Helper()
	out := &syncBuilder{}
	if err := runStatuslineWith(t, out, noServer, payload, args...); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestClaudeStatuslineReport_WindowsSent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, payload string
		five, seven   bool
		sent          bool
	}{
		{"both", usagePayload, true, true, true},
		{"one", `{"rate_limits":{"seven_day":{"used_percentage":9,"resets_at":1790500000}}}`, false, true, true},
		{"none", `{"model":{"display_name":"Claude"}}`, false, false, false},
		{"malformed", `{not json`, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := &syncBuilder{}
			rs, cl := startReportServer(t, out, nil)
			dial := func() (*apiclient.Client, error) { return cl, nil }
			_ = runStatuslineWith(t, out, dial, tc.payload)
			if got := rs.count() > 0; got != tc.sent {
				t.Fatalf("sent = %v, want %v", got, tc.sent)
			}
			if !tc.sent {
				return
			}
			if rs.count() != 1 {
				t.Fatalf("reports = %d, want 1", rs.count())
			}
			rep := rs.reports[0]
			if (rep.FiveHour != nil) != tc.five || (rep.SevenDay != nil) != tc.seven {
				t.Errorf("report = %+v, want five=%v seven=%v", rep, tc.five, tc.seven)
			}
		})
	}
}

// --install returns before any report; it is not run here because it writes
// the real Claude settings file.
func TestClaudeStatuslineReport_DemoSendsNothing(t *testing.T) {
	t.Parallel()
	out := &syncBuilder{}
	rs, cl := startReportServer(t, out, nil)
	dial := func() (*apiclient.Client, error) { return cl, nil }
	_ = runStatuslineWith(t, out, dial, usagePayload, "--demo")
	if rs.count() != 0 {
		t.Errorf("reports = %d, want 0", rs.count())
	}
}

func TestClaudeStatuslineReport_LinePrintedBeforeReport(t *testing.T) {
	t.Parallel()
	out := &syncBuilder{}
	rs, cl := startReportServer(t, out, nil)
	if err := runStatuslineWith(t, out, func() (*apiclient.Client, error) { return cl, nil }, usagePayload); err != nil {
		t.Fatal(err)
	}
	if rs.count() != 1 || !rs.lineSeen[0] {
		t.Errorf("count = %d, lineSeen = %v; want the line printed before the request", rs.count(), rs.lineSeen)
	}
}

func TestClaudeStatuslineReport_SameOutputWhateverTheServerDoes(t *testing.T) {
	t.Parallel()
	want := baseline(t, usagePayload)
	for name, status := range map[string]int{"500": 500, "404": 404} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := &syncBuilder{}
			_, cl := startReportServer(t, out, func(w http.ResponseWriter, _ *http.Request, _ *reportServer) {
				http.Error(w, "nope", status)
			})
			if err := runStatuslineWith(t, out, func() (*apiclient.Client, error) { return cl, nil }, usagePayload); err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if out.String() != want {
				t.Errorf("output = %q, want %q", out.String(), want)
			}
		})
	}
}

func TestClaudeStatuslineReport_HungServerBoundedByTimeout(t *testing.T) {
	t.Parallel()
	want := baseline(t, usagePayload)
	out := &syncBuilder{}
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	_, cl := startReportServer(t, out, func(_ http.ResponseWriter, r *http.Request, _ *reportServer) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	})
	start := time.Now()
	if err := runStatuslineWith(t, out, func() (*apiclient.Client, error) { return cl, nil }, usagePayload); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if took := time.Since(start); took > statuslineReportTimeout+400*time.Millisecond {
		t.Errorf("took %v, want about the %v timeout", took, statuslineReportTimeout)
	}
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestClaudeStatuslineReport_AgentDirRecordAndReport(t *testing.T) {
	t.Parallel()
	out := &syncBuilder{}
	rs, cl := startReportServer(t, out, nil)
	dir := t.TempDir()
	if err := runStatuslineWith(t, out, func() (*apiclient.Client, error) { return cl, nil }, usagePayload, "--agent-dir", dir); err != nil {
		t.Fatal(err)
	}
	if rs.count() != 1 {
		t.Errorf("reports = %d, want 1", rs.count())
	}
	if _, err := os.Stat(filepath.Join(dir, nativerunner.UsageFile)); err != nil {
		t.Errorf("usage record: %v", err)
	}
}
