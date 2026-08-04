package ralphloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event type strings recorded in an epic's run-log.jsonl.
const (
	eventIterationStarted        = "iteration-started"
	eventIterationFinished       = "iteration-finished"
	eventCherryPicked            = "cherry-picked"
	eventConflictHit             = "conflict-hit"
	eventConflictResolved        = "conflict-resolved"
	eventPausedSmartZone         = "paused-smart-zone"
	eventSmartZoneRecoveryFailed = "smart-zone-recovery-failed"
	// eventSmartZoneWaitExpired marks a compact-recovery wait that expired
	// past smartZoneCompactTimeoutMs without herdr's pane-status wait ever
	// confirming completion, but where the transcript's compaction-boundary
	// signal showed compaction actually finished anyway — a slower-than-usual
	// compact, not a failure, and deliberately distinct from
	// eventSmartZoneRecoveryFailed so it isn't misread as one.
	eventSmartZoneWaitExpired = "smart-zone-wait-expired"
	eventPausedRateLimit         = "paused-rate-limit"
	eventResumed                 = "resumed"
	eventNeedsInfo               = "needs-info"
	eventCommitless              = "commitless"
	eventNeedsAttention          = "needs-attention"
	eventDepsInstalled           = "deps-installed"
)

// Event is one line of an epic's run-log.jsonl: a single lifecycle
// occurrence, timestamped and attributed to the ticket/pane/tab it happened
// on. AgentSession is empty for events not tied to a specific agent session
// (e.g. conflict-hit, which precedes the conflict-resolution agent's own
// launch). Agent is omitted by historical logs and defaults to Claude when
// reports read them. Reason carries the reason for pause and attention
// events, or the install command run (empty if none matched) for
// deps-installed events.
type Event struct {
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	// Ticket is the ticket's Identifier (the filename's full "NN[letter]"
	// prefix), not its Number, so lettered split siblings sharing a Number
	// (e.g. "04a"/"04b") remain distinguishable in the run log.
	Ticket       string    `json:"ticket"`
	Agent        AgentKind `json:"agent,omitempty"`
	Pane         string    `json:"pane,omitempty"`
	Tab          string    `json:"tab,omitempty"`
	AgentSession string    `json:"agent_session,omitempty"`
	// Cwd is the directory the agent session was launched in and is the local
	// session-data lookup key alongside AgentSession.
	Cwd    string `json:"cwd,omitempty"`
	Reason string `json:"reason,omitempty"`
	// SHA is the feature branch's tip commit right after a cherry-picked
	// event landed a ticket's iteration. Recorded because CherryPickRange
	// creates fresh commits (different hashes than the iteration branch's
	// originals), so it's the only durable record startup reconciliation can
	// check for reachability from the feature branch's later tip — the
	// iteration branch itself isn't guaranteed to still exist by then.
	SHA string `json:"sha,omitempty"`
}

// eventLogMu serializes appends across every goroutine in the process (each
// running iteration logs concurrently), so two events never interleave
// their JSON onto the same line.
var eventLogMu sync.Mutex

// runLogPath returns the append-only event log path for epicName under
// scratchDir.
func runLogPath(scratchDir, epicName string) string {
	return filepath.Join(scratchDir, epicName, "run-log.jsonl")
}

// logEvent appends ev as one JSON line to epicName's run-log.jsonl under
// scratchDir, creating the epic directory if it doesn't exist yet, and
// filling Time in if it's zero. It's a no-op if scratchDir or epicName is
// empty, so call sites that don't have logging wired up (e.g. the
// conflict-resolution launch, which logs conflict-hit/resolved manually
// instead of via the generic start/finish path) don't need to special-case
// it.
func logEvent(scratchDir, epicName string, ev Event) error {
	if scratchDir == "" || epicName == "" {
		return nil
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := runLogPath(scratchDir, epicName)

	eventLogMu.Lock()
	defer eventLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// lastIterationSession finds the most recent iteration-started event for
// identifier (a ticket's Identifier, not Number, so lettered split siblings
// aren't cross-attributed) with a recorded AgentSession — the session
// id/cwd/agent to backfill a reattached ticket close's metadata with (ticket
// 06a), since the reattaching run never captured a fresh session of its own.
// Agent defaults to AgentClaude for historical logs that omitted it (see
// Event.Agent).
func lastIterationSession(events []Event, identifier string) (agentSession, cwd string, agent AgentKind, ok bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Ticket != identifier || ev.Type != eventIterationStarted || ev.AgentSession == "" {
			continue
		}
		agent = ev.Agent
		if agent == "" {
			agent = AgentClaude
		}
		return ev.AgentSession, ev.Cwd, agent, true
	}
	return "", "", "", false
}

// readEvents reads and parses every line of epicName's run-log.jsonl under
// scratchDir, skipping malformed lines rather than failing the whole read
// (a run-log written by a process killed mid-write may have a torn final
// line). ok is false if the log doesn't exist yet.
func readEvents(scratchDir, epicName string) (events []Event, ok bool, err error) {
	raw, err := os.ReadFile(runLogPath(scratchDir, epicName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, true, nil
}
