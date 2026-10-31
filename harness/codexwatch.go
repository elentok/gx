package harness

import (
	"encoding/json"
	"os"
	"sync"
)

// codexBackgroundTitle is the title Codex shows while it works with nothing
// in the foreground, a background command still running.
const codexBackgroundTitle = "Ready"

// isSpinner reports whether r is a braille pattern, which Codex's title
// spinner draws with.
func isSpinner(r rune) bool { return r >= 0x2800 && r <= 0x28FF }

// codexWatcher follows a Codex session's OSC 7501 reports, with the title as
// the fallback until the first report. Status is derived on demand from the
// last report and the title, so the order they arrive in does not matter.
type codexWatcher struct {
	sessionFile string
	// startup reports a startup dialog on screen.
	startup func() bool

	mu sync.Mutex
	// state, kind and message are the last report other than clear; state is
	// "" before the first.
	state, kind, message string
	title                string
	haveTitle            bool
	idleSeen             bool
	// turns counts working reports, so a task that starts while a background
	// terminal already holds the state at working still counts.
	turns int
}

func newCodexWatcher(sessionFile string, startup func() bool) *codexWatcher {
	return &codexWatcher{sessionFile: sessionFile, startup: startup}
}

func (w *codexWatcher) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch w.state {
	case "":
		if w.startup() {
			return Status{State: Blocked, Blocked: BlockedDialog}
		}
		switch {
		case !w.haveTitle:
			return Status{State: Starting}
		case spinnerPrefix(w.title):
			return Status{State: Working}
		}
		return Status{State: Idle}
	case "working":
		return Status{State: Working, Background: w.title == codexBackgroundTitle}
	case "blocked":
		kind := BlockedQuestion
		if w.kind == string(BlockedPermission) {
			kind = BlockedPermission
		}
		return Status{State: Blocked, Blocked: kind, Message: w.message}
	case "error":
		// The server reads any Blocked status as a blocked pane.
		return Status{State: Blocked, Blocked: BlockedDialog, Message: w.message}
	}
	return Status{State: Idle}
}

// ConversationID is the session_id the SessionStart hook saved. It rereads the
// file each time, so a resumed session replaces a stale id.
func (w *codexWatcher) ConversationID() string {
	if w.sessionFile == "" {
		return ""
	}
	data, err := os.ReadFile(w.sessionFile)
	if err != nil {
		return ""
	}
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return ""
	}
	return payload.SessionID
}

// Program ignores clear: Codex sends it on exit, and Exited comes from the
// process's death.
func (w *codexWatcher) Program(r ProgramReport) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch r.State {
	case "working", "blocked", "error":
	case "idle", "done":
		w.idleSeen = w.idleSeen || r.State == "idle"
	default:
		return
	}
	if r.State == "working" {
		w.turns++
	}
	w.state, w.kind, w.message = r.State, r.Kind, r.Message
}

// Turns is the number of working reports seen.
func (w *codexWatcher) Turns() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.turns
}

func (w *codexWatcher) Title(title string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.title, w.haveTitle = title, true
}

// OnChange never calls f: the session file is written before Codex first
// reports idle, and output drives every later change.
func (w *codexWatcher) OnChange(func()) {}

// Ready is the first idle report. Codex shows its startup dialogs first, so
// they are answered by then.
func (w *codexWatcher) Ready() ReadySignal {
	if w.Status().State == Blocked {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.idleSeen {
		return ReadyOSC7501
	}
	return ""
}

var (
	_ TitleWatcher = (*codexWatcher)(nil)
	_ TurnWatcher  = (*codexWatcher)(nil)
)

// spinnerPrefix reports whether title starts with a spinner glyph.
func spinnerPrefix(title string) bool {
	for _, r := range title {
		return isSpinner(r)
	}
	return false
}
