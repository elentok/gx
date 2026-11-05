// Package runlog is the append-only run log (run-log.jsonl) and the EventSink
// interface the scheduler reports lifecycle events through. It imports agent
// only among ralphloop packages.
package runlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/tickets"
)

// Decision is a ticket's scheduling disposition in one scheduler scan.
type Decision string

const (
	DecisionClaimed    Decision = "claimed"
	DecisionOutOfScope Decision = "out-of-scope"
	DecisionDone       Decision = "done"
	DecisionError      Decision = "error"
	DecisionStalled    Decision = "stalled"
	DecisionFrontier   Decision = "frontier"
	DecisionBlocked    Decision = "blocked"
	DecisionUnclaimed  Decision = "unclaimed"
)

// ScanDecision records one ticket's scheduling disposition for a single
// scheduler-scan log line, in Epic.Tickets order — every ticket
// loadNamedEpic returned at the moment claimNext evaluated the frontier, not
// just the ones in scope or in the frontier, so a ticket that's silently
// out-of-scope (the most common cause of "queued but never starts") still
// shows up.
type ScanDecision struct {
	// Ticket is the ticket's Identifier (see Event.Ticket).
	Ticket string `json:"ticket"`
	// Status is the ticket's RenderedStatus (see tickets.RenderedStatus.Word).
	Status string `json:"status"`
	// Decision is one of: "claimed" (this pass claimed it), "frontier"
	// (open/unblocked/in-scope but a different ticket was claimed instead,
	// or maxParallel was already reached elsewhere in this Run call),
	// "out-of-scope" (RunScope.Contains is false — e.g. a code-review
	// ticket's child missing its own parent: back-edge), "blocked", or
	// "settled" (already done/paused (answer)/paused (repair)).
	Decision Decision `json:"decision"`
	// Reason elaborates Decision — e.g. UnresolvedBlockers's tokens for
	// "blocked". Empty when Decision is self-explanatory.
	Reason string `json:"reason,omitempty"`
}

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
	Ticket       string          `json:"ticket"`
	Agent        agent.AgentKind `json:"agent,omitempty"`
	Pane         string          `json:"pane,omitempty"`
	AgentSession string          `json:"agent_session,omitempty"`
	// Cwd is the directory the agent session was launched in and is the local
	// session-data lookup key alongside AgentSession.
	Cwd    string `json:"cwd,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Text is what was typed into the pane (nudged) or the exact verbs a
	// recovery proposes, one per line (recovery-proposed).
	Text string `json:"text,omitempty"`
	// Fingerprint (recovery-proposed only) is the ticket file's hash right after
	// the proposal was written; approve refuses a proposal once it differs.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Signature (recovery-matched only) is the matched catalog entry's
	// signature; Reason holds its ID.
	Signature string `json:"signature,omitempty"`
	// Kind (failure and recovery events) is the closed-enum cause from the
	// events package; required on the types events.KindRequired names.
	Kind string `json:"kind,omitempty"`
	// Label is the iteration label (launch-failed, prompt-nudged and pause events).
	Label string `json:"label,omitempty"`
	// Runner (pause events) names the agentrunner that hosted the agent: a pause
	// drops the runs.json entry, so late-finish adoption finds the agent here.
	Runner string `json:"runner,omitempty"`
	// Model and Effort (pause events) are what the agent launched under, so an
	// adopted late finish merges under the launch model, not the project default.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Address is the ticket's canonical address ("project:epic/06"), filled by
	// AppendEvent from Ticket when empty.
	Address string `json:"address,omitempty"`
	// Iteration/Attempt are set only when known/applicable, never as
	// placeholders.
	Iteration int `json:"iteration,omitempty"`
	Attempt   int `json:"attempt,omitempty"`
	// SHA is the feature branch's tip commit right after a cherry-picked
	// event merged a ticket's iteration. Recorded because CherryPickRange
	// creates fresh commits (different hashes than the iteration branch's
	// originals), so it's the only durable record startup reconciliation can
	// check for reachability from the feature branch's later tip — the
	// iteration branch itself isn't guaranteed to still exist by then.
	SHA string `json:"sha,omitempty"`
	// ResolvedBase (iteration-started only) is the base (ref + SHA) the claim
	// resolved; `gx tickets validate` compares it with the ticket's stamp.
	ResolvedBase string `json:"resolved_base,omitempty"`
	// TargetTip (iteration-started of a merge ticket only) is the SHA of the
	// merge target when the ticket was claimed; a finish accepts an epic
	// rebased onto it even after the target moves on.
	TargetTip string `json:"target_tip,omitempty"`
	// Path (iteration-started only, from the server's claim) is how the agent
	// began: "resumed" on its earlier conversation, else "cold-start".
	Path string `json:"path,omitempty"`
	// Scan (scheduler-scan events only) is every ticket's disposition for
	// this claimNext pass — see ScanDecision.
	Scan []ScanDecision `json:"scan,omitempty"`
	// Telegram/Slack (notifications-configured only) record whether each
	// channel is active for this run — never the token/webhook values
	// themselves. Pointers (rather than plain bool) so "false" still
	// serializes explicitly instead of vanishing under omitempty.
	Telegram *bool `json:"telegram,omitempty"`
	Slack    *bool `json:"slack,omitempty"`
	// Channel (notification-sent/notification-failed only) is which channel
	// the attempt used: "telegram" or "slack".
	Channel string `json:"channel,omitempty"`
	// NotifyKind (notification-sent/notification-failed only) is the live
	// event that triggered the send attempt — see the notifyKind* consts in
	// package ralphloop.
	NotifyKind string `json:"notify_kind,omitempty"`
	// Body (notification-sent/notification-failed only) is the message text
	// that was sent (or attempted), so counter/content bugs are diagnosable
	// from run-log.jsonl alone without reproducing the send.
	Body string `json:"body,omitempty"`
	// StateChangeSeq (iteration-started only, and only when the agent was
	// actually launched here rather than attached to) is herdr's
	// turn count at the moment the session started — the launch-time
	// baseline a later adopted launch (adoptedLaunch) compares its own
	// live reading against to tell "idle because it genuinely finished a
	// turn" from "idle because it never left this launch state" (see
	// noActivitySinceLaunch).
	StateChangeSeq int `json:"state_change_seq,omitempty"`
	// Outcome (manual-merge/ticket-started-over, recovery-applied) is what the
	// recovery command or remedy concluded: "ok" or the remedy's error.
	Outcome string `json:"outcome,omitempty"`
	// TrailerValue (manual-merge/ticket-started-over only) is the trailer value the
	// command matched or wrote on the merged commits.
	TrailerValue string `json:"trailer_value,omitempty"`
	// AtticRef (manual-merge/ticket-started-over only) is the ref that preserves the
	// commits the command set aside.
	AtticRef string `json:"attic_ref,omitempty"`
	// BackupRef (dirty-trunk-landed only) is the gx ref holding the files the
	// land set aside.
	BackupRef string `json:"backup_ref,omitempty"`
	// TaskID (background-task gate events only) is the gated task, so readers
	// never parse it out of Reason.
	TaskID string `json:"task_id,omitempty"`
	// Session, Source, Action and Bytes (session-prompted only) name the
	// session, where the send came from (web or cli), what it did (text) and
	// how many bytes it typed.
	Session string `json:"session,omitempty"`
	Source  string `json:"source,omitempty"`
	Action  string `json:"action,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
}

// eventLogMu serializes appends across every goroutine in the process (each
// running iteration logs concurrently), so two events never interleave
// their JSON onto the same line.
var eventLogMu sync.Mutex

// RunLogPath returns the append-only event log path for epicName under
// scratchDir.
func RunLogPath(scratchDir, epicName string) string {
	return filepath.Join(scratchDir, epicName, "run-log.jsonl")
}

// AppendEvent appends ev as one JSON line to epicName's run-log.jsonl under
// scratchDir, filling Time in if it's zero. It's a no-op if scratchDir or
// epicName is empty, so call sites that don't have logging wired up (e.g. the
// conflict-resolution launch, which logs conflict-hit/resolved manually
// instead of via the generic start/finish path) don't need to special-case
// it. It never creates a missing epic directory: one made that way has no
// ticket.md, so it would be an invalid epic. The event is dropped instead.
//
// There is no on-disk lock: concurrent processes (one-shot CLI callers
// alongside a live ralph-loop) rely on O_APPEND atomicity of a single write
// under events.MaxLineBytes, which every append enforces.
func AppendEvent(scratchDir, epicName string, ev Event) error {
	if scratchDir == "" || epicName == "" {
		return nil
	}
	if ev.Ticket != "" && ev.Address == "" {
		ev.Address = tickets.Address{Project: tickets.ProjectName(scratchDir), Epic: epicName, ID: ev.Ticket}.String()
	}
	data, err := encodeEvent(&ev)
	if err != nil {
		return err
	}
	err = appendLine(RunLogPath(scratchDir, epicName), data)
	if errors.Is(err, fs.ErrNotExist) {
		logger.Debug("dropping %s event: epic %s has no directory\n", ev.Type, epicName)
		return nil
	}
	return err
}

// encodeEvent stamps ev's Time, validates its type/kind and marshals it to one
// log line that fits events.MaxLineBytes.
func encodeEvent(ev *Event) ([]byte, error) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if err := events.Validate(events.Type(ev.Type), events.Kind(ev.Kind)); err != nil {
		return nil, err
	}
	return events.Fit(func() ([]byte, error) { return json.Marshal(ev) }, &ev.Reason, &ev.Body)
}

// serverLogPath returns the server event log: one log per store, outside any
// project, for events that have no address.
func serverLogPath(storeDir string) string {
	return filepath.Join(storeDir, "server-log.jsonl")
}

// AppendServerEvent appends an address-less event (herdr availability, budget
// latches, server crash/restart) to the store's server event log. Epic events
// never go through here: an event naming a ticket belongs in its epic's log.
func AppendServerEvent(storeDir string, ev Event) error {
	if storeDir == "" {
		return nil
	}
	if ev.Ticket != "" || ev.Address != "" {
		return fmt.Errorf("event %q has an address; it belongs in an epic's log", ev.Type)
	}
	data, err := encodeEvent(&ev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return err
	}
	return appendLine(serverLogPath(storeDir), data)
}

// appendLine writes data plus a newline to path. Its directory must exist.
func appendLine(path string, data []byte) error {
	data = append(data, '\n')

	eventLogMu.Lock()
	defer eventLogMu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// IterationSession is the session one iteration-started event recorded.
type IterationSession struct {
	Session string
	Cwd     string
	Agent   agent.AgentKind
}

// IterationSessions lists identifier's recorded iteration sessions, newest
// first. Agent defaults to agent.AgentClaude for historical logs that omitted it.
func IterationSessions(evs []Event, identifier string) []IterationSession {
	var out []IterationSession
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.Ticket != identifier || ev.Type != string(events.IterationStarted) || ev.AgentSession == "" {
			continue
		}
		agentKind := ev.Agent
		if agentKind == "" {
			agentKind = agent.AgentClaude
		}
		out = append(out, IterationSession{Session: ev.AgentSession, Cwd: ev.Cwd, Agent: agentKind})
	}
	return out
}

// ReadEvents reads and parses every line of epicName's run-log.jsonl under
// scratchDir, skipping malformed lines rather than failing the whole read
// (a run-log written by a process killed mid-write may have a torn final
// line). ok is false if the log doesn't exist yet.
func ReadEvents(scratchDir, epicName string) (events []Event, ok bool, err error) {
	return readEventsFile(RunLogPath(scratchDir, epicName))
}

// ReadServerEvents reads the store's server event log, like ReadEvents.
func ReadServerEvents(storeDir string) (events []Event, ok bool, err error) {
	return readEventsFile(serverLogPath(storeDir))
}

func readEventsFile(path string) (evs []Event, ok bool, err error) {
	raw, err := os.ReadFile(path)
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
		t, k := events.ReadLegacy(events.Type(ev.Type), events.Kind(ev.Kind))
		ev.Type, ev.Kind = string(t), string(k)
		evs = append(evs, ev)
	}
	return evs, true, nil
}

// LatestPauseEvent is the ticket's most recent pause event; false when the run
// log has none.
func LatestPauseEvent(scratchDir, epic, ticket string) (Event, bool) {
	evs, ok, err := ReadEvents(scratchDir, epic)
	if err != nil || !ok {
		return Event{}, false
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Ticket == ticket && events.Type(evs[i].Type) == events.Paused {
			return evs[i], true
		}
	}
	return Event{}, false
}
