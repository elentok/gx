package harness

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/codexsession"
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
	// home is the Codex home the rollouts live under; "" reads none.
	home string
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
		return w.errorStatus()
	}
	return Status{State: Idle}
}

// errorStatus sorts a turn error by its cause: a usage limit is RateLimited
// with its reset, any other error (or one the rollout cannot show) is Blocked
// with Codex's own text. The server reads any Blocked status as a blocked pane.
func (w *codexWatcher) errorStatus() Status {
	status := Status{State: Blocked, Blocked: BlockedError, Message: w.message}
	payload := w.sessionPayload()
	if w.home == "" || payload.SessionID == "" {
		return status
	}
	reading, ok, err := codexsession.LastTurnErrorIn(w.home, payload.Cwd, payload.SessionID)
	if err != nil || !ok {
		return status
	}
	if limit, isLimit := reading.UsageLimit(time.Now()); isLimit {
		return Status{State: RateLimited, Message: rateLimitMessage(limit.ResetAt), ResetAt: limit.ResetAt}
	}
	if reading.Error.Message != "" {
		status.Message = reading.Error.Message
	}
	return status
}

func rateLimitMessage(resetAt time.Time) string {
	if resetAt.IsZero() {
		return "usage limit reached"
	}
	return "resets " + resetAt.Local().Format("Mon Jan 2 15:04")
}

// sessionPayload is the SessionStart hook's saved payload. It rereads the file
// each time, so a resumed session replaces a stale id.
func (w *codexWatcher) sessionPayload() (payload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}) {
	if w.sessionFile == "" {
		return payload
	}
	data, err := os.ReadFile(w.sessionFile)
	if err != nil || json.Unmarshal(data, &payload) != nil {
		return payload
	}
	return payload
}

// ConversationID is the session_id the SessionStart hook saved.
func (w *codexWatcher) ConversationID() string {
	return w.sessionPayload().SessionID
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
