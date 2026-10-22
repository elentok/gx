package tickets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/tickets/schema"
)

// Pause event types in a ticket's log.
const (
	EventPauseQuestion   = "pause.question"
	EventPauseAnswer     = "pause.answer"
	EventPausePermission = "pause.permission"
	EventPauseClosed     = "pause.closed"
)

// legacyEventTypes maps the event types logs used to store to their current
// spelling. readLog folds them on read, so a log is never rewritten.
var legacyEventTypes = map[string]string{
	"park.question":   EventPauseQuestion,
	"park.answer":     EventPauseAnswer,
	"park.permission": EventPausePermission,
	"park.closed":     EventPauseClosed,
}

const (
	// MaxLogTextChars caps every free-text field of a log event.
	MaxLogTextChars = 4000
	// MaxLogSummaryChars caps a pause.question summary.
	MaxLogSummaryChars = 120
)

// PermissionShown is what the person saw when a permission prompt paused the ticket.
type PermissionShown struct {
	RequestID   string `json:"request_id,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Input       string `json:"input,omitempty"`
	Dialog      string `json:"dialog,omitempty"`
	Description string `json:"description,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// LogEvent is one line of a ticket's "<NN-slug>.jsonl" log. Which fields apply
// depends on Type; the rest stay empty and are omitted from the line.
type LogEvent struct {
	Seq  int       `json:"seq"`
	TS   time.Time `json:"ts"`
	Type string    `json:"type"`

	// pause.question
	By        string `json:"by,omitempty"`
	Session   string `json:"session,omitempty"`
	PauseKind string `json:"pause_kind,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Text      string `json:"text,omitempty"`
	Block     string `json:"block,omitempty"`
	Handoff   string `json:"handoff,omitempty"`

	// LegacyKind is the old spelling of PauseKind. readLog moves it into
	// PauseKind, so it is empty on every event a caller sees.
	LegacyKind string `json:"park_kind,omitempty"`

	// pause.answer, pause.permission and pause.closed
	Question int              `json:"question,omitempty"`
	Source   string           `json:"source,omitempty"`
	Choice   string           `json:"choice,omitempty"`
	Note     string           `json:"note,omitempty"`
	Decision string           `json:"decision,omitempty"`
	Shown    *PermissionShown `json:"shown,omitempty"`
	Reason   string           `json:"reason,omitempty"`
	Base     string           `json:"base,omitempty"`
}

// ClipLogText cuts text to MaxLogTextChars and says whether it cut anything.
func ClipLogText(text string) (string, bool) {
	clipped := clipRunes(text, MaxLogTextChars)
	return clipped, clipped != text
}

// LogPath is the log beside the ticket file at ticketPath.
func LogPath(ticketPath string) string {
	return strings.TrimSuffix(ticketPath, ".md") + ".jsonl"
}

// AppendLog adds ev to the log of the ticket at ticketPath. Under the ticket
// lock it assigns Seq as the last seq plus one, stamps TS when zero, drops a
// torn last line left by a crashed writer, and writes the whole line in one
// write() on an O_APPEND file. It returns the event as written.
func AppendLog(ticketPath string, ev LogEvent) (LogEvent, error) {
	return AppendLogIf(ticketPath, ev, nil)
}

// AppendLogIf is AppendLog with a guard that sees the existing events under the
// ticket lock. A guard error writes nothing and is returned as is, so a
// check-then-append cannot race another writer.
func AppendLogIf(ticketPath string, ev LogEvent, guard func([]LogEvent) error) (LogEvent, error) {
	return appendLogWith(ticketPath, func(evs []LogEvent) (LogEvent, error) {
		if guard != nil {
			if err := guard(evs); err != nil {
				return LogEvent{}, err
			}
		}
		return ev, nil
	})
}

// AppendLogFrom appends the event mk builds from the existing events, all under
// the ticket lock. An error from mk writes nothing and is returned as is.
func AppendLogFrom(ticketPath string, mk func([]LogEvent) (LogEvent, error)) (LogEvent, error) {
	return appendLogWith(ticketPath, mk)
}

// appendLogWith appends the event mk builds from the existing events, all under
// the ticket lock.
func appendLogWith(ticketPath string, mk func([]LogEvent) (LogEvent, error)) (LogEvent, error) {
	unlock, err := schema.LockTicket(ticketPath)
	if err != nil {
		return LogEvent{}, err
	}
	defer unlock()

	logPath := LogPath(ticketPath)
	events, validLen, err := readLog(logPath)
	if err != nil {
		return LogEvent{}, err
	}
	ev, err := mk(events)
	if err != nil {
		return LogEvent{}, err
	}
	if err := validateLogEvent(ev); err != nil {
		return LogEvent{}, err
	}
	if err := os.Truncate(logPath, validLen); err != nil && !os.IsNotExist(err) {
		return LogEvent{}, fmt.Errorf("trimming torn line of %s: %w", logPath, err)
	}

	ev.Seq = 1
	if n := len(events); n > 0 {
		ev.Seq = events[n-1].Seq + 1
	}
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}

	line, err := json.Marshal(ev)
	if err != nil {
		return LogEvent{}, err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return LogEvent{}, err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return LogEvent{}, fmt.Errorf("appending to %s: %w", logPath, err)
	}
	return ev, nil
}

// ReadLog returns the events of the ticket's log in order. A missing log is
// empty. An unparseable last line without a trailing newline is a torn write
// and is skipped; any other unparseable line is an error naming the line.
func ReadLog(ticketPath string) ([]LogEvent, error) {
	events, _, err := readLog(LogPath(ticketPath))
	return events, err
}

// readLog also returns the byte length of the intact prefix, so a writer can
// cut a torn tail off.
func readLog(logPath string) ([]LogEvent, int64, error) {
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}

	var events []LogEvent
	var validLen int64
	for lineNo, rest := 1, data; len(rest) > 0; lineNo++ {
		line := rest
		terminated := false
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			line, rest, terminated = rest[:i], rest[i+1:], true
		} else {
			rest = nil
		}

		var ev LogEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			if !terminated {
				break
			}
			return nil, 0, fmt.Errorf("%s line %d: malformed event: %w", logPath, lineNo, err)
		}
		if t, ok := legacyEventTypes[ev.Type]; ok {
			ev.Type = t
		}
		if ev.PauseKind == "" {
			ev.PauseKind = ev.LegacyKind
		}
		ev.LegacyKind = ""
		events = append(events, ev)
		validLen += int64(len(line))
		if terminated {
			validLen++
		}
	}
	return events, validLen, nil
}

func validateLogEvent(ev LogEvent) error {
	if ev.Type == "" {
		return fmt.Errorf("log event has no type")
	}
	if n := utf8.RuneCountInString(ev.Summary); n > MaxLogSummaryChars {
		return fmt.Errorf("summary is %d characters, max %d", n, MaxLogSummaryChars)
	}
	texts := map[string]string{
		"text": ev.Text, "handoff": ev.Handoff, "block": ev.Block, "note": ev.Note,
	}
	if s := ev.Shown; s != nil {
		texts["shown.input"] = s.Input
		texts["shown.dialog"] = s.Dialog
		texts["shown.description"] = s.Description
	}
	for name, text := range texts {
		if n := utf8.RuneCountInString(text); n > MaxLogTextChars {
			return fmt.Errorf("%s is %d characters, max %d", name, n, MaxLogTextChars)
		}
	}
	if ev.Type == EventPauseQuestion && ev.By == "agent" && ev.Handoff == "" {
		return fmt.Errorf("pause.question by agent needs a handoff")
	}
	return nil
}

var errNoOpenQuestion = errors.New("no open question")

// CloseOpenQuestion writes pause.closed{question, reason} for the ticket's open
// question, for a path that leaves the paused state without a person's answer. A
// ticket with no open question (a pre-migration one) gets no write and no error.
func CloseOpenQuestion(ticketPath, reason string) error {
	_, err := appendLogWith(ticketPath, func(evs []LogEvent) (LogEvent, error) {
		open := OpenQuestion(evs)
		if open == nil {
			return LogEvent{}, errNoOpenQuestion
		}
		return LogEvent{Type: EventPauseClosed, Question: open.Seq, Reason: reason}, nil
	})
	if errors.Is(err, errNoOpenQuestion) {
		return nil
	}
	return err
}

// PauseRound is a pause.question and the event that ended it, if any.
type PauseRound struct {
	Question LogEvent
	// End is the pause.answer or pause.closed naming Question, nil while open.
	End *LogEvent
}

// Rounds pairs each pause.question with the pause.answer or pause.closed that
// names its seq. Other events (pause.permission) belong to no round.
func Rounds(events []LogEvent) []PauseRound {
	var rounds []PauseRound
	bySeq := map[int]int{}
	for _, ev := range events {
		switch ev.Type {
		case EventPauseQuestion:
			bySeq[ev.Seq] = len(rounds)
			rounds = append(rounds, PauseRound{Question: ev})
		case EventPauseAnswer, EventPauseClosed:
			i, ok := bySeq[ev.Question]
			if ok && rounds[i].End == nil {
				end := ev
				rounds[i].End = &end
			}
		}
	}
	return rounds
}

// OpenQuestion is the latest pause.question with neither an answer nor a close,
// or nil.
func OpenQuestion(events []LogEvent) *LogEvent {
	rounds := Rounds(events)
	if n := len(rounds); n > 0 && rounds[n-1].End == nil {
		q := rounds[n-1].Question
		return &q
	}
	return nil
}
