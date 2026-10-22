package agents

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
)

// FocusClient names this TUI to the server: its event stream subscribes under
// it and its focus reports carry it, so the reports expire with the stream.
var FocusClient = "tui-" + strconv.Itoa(os.Getpid())

const focusReportTimeout = 5 * time.Second

// LastViewedFile is where the tab remembers the session viewed last, so a
// new TUI opens on it; empty when the state dir is unknown.
func LastViewedFile() string {
	dir, err := config.StateDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agents-last-viewed")
}

// WithLastViewedFile makes the tab open on the session saved in path and save
// each session it shows there.
func (m Model) WithLastViewedFile(path string) Model {
	if path == "" {
		return m
	}
	if data, err := os.ReadFile(path); err == nil {
		m.lastViewed = strings.TrimSpace(string(data))
	}
	if r := m.reporter; r != nil {
		r.mu.Lock()
		r.lastViewedPath = path
		r.mu.Unlock()
	}
	return m
}

// open picks the opening focus: the terminal on the session viewed last, else
// on the first one needing attention; else the sidebar, which shows the
// new-session hint when there are no sessions.
func (m Model) open() Model {
	i := slices.IndexFunc(m.sessions, func(s server.SessionInfo) bool { return s.ID == m.lastViewed })
	if i < 0 {
		i = m.attentionFrom(0)
	}
	if i < 0 {
		return m
	}
	m.cursor = i
	m.terminalFocused = m.attachTarget() != ""
	return m
}

func (m Model) focusReport() server.FocusReport {
	r := server.FocusReport{Client: FocusClient, Focused: m.focused, TermProgram: m.termProgram}
	if m.lines() == nil {
		r.Session = m.selectedID()
	}
	return r
}

// reportFocus tells the server what the tab shows; called after every update
// because any of them can change it.
func (m Model) reportFocus() { m.reporter.report(m.focusReport()) }

// focusReporter sends focus reports in order, off the update loop, dropping
// any superseded before it was sent.
type focusReporter struct {
	client SessionClient

	mu             sync.Mutex
	lastViewedPath string
	// last is the newest report; pending: it has not been sent yet.
	last    server.FocusReport
	pending bool
	sending bool
}

func (r *focusReporter) report(rep server.FocusReport) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rep == r.last {
		return
	}
	r.last, r.pending = rep, true
	if !r.sending {
		r.sending = true
		go r.flushPending()
	}
}

func (r *focusReporter) flushPending() {
	for {
		r.mu.Lock()
		if !r.pending {
			r.sending = false
			r.mu.Unlock()
			return
		}
		rep, path := r.last, r.lastViewedPath
		r.pending = false
		r.mu.Unlock()
		r.send(rep, path)
	}
}

func (r *focusReporter) send(rep server.FocusReport, lastViewedPath string) {
	if rep.Session != "" && lastViewedPath != "" {
		_ = os.MkdirAll(filepath.Dir(lastViewedPath), 0o755)
		_ = os.WriteFile(lastViewedPath, []byte(rep.Session+"\n"), 0o644)
	}
	ctx, cancel := context.WithTimeout(context.Background(), focusReportTimeout)
	defer cancel()
	res, err := r.client.ReportFocus(ctx, rep)
	if err == nil && !res.Refused {
		return
	}
	// A report sent before the event stream opened is refused; forgetting it
	// makes the next update send it again.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == rep {
		r.last = server.FocusReport{}
	}
}
