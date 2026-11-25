package transcript

import (
	"time"
)

// BackgroundTaskStatus is one of the five states ReadBackgroundTasks can
// report, either as a marker's own resolution state or (Unreadable/
// Unsupported only) as BackgroundTaskReading.FileStatus when the whole read
// failed before any marker could be resolved.
type BackgroundTaskStatus string

const (
	// BackgroundTaskResolved means a task-notification matching the marker's
	// task id was found later in the transcript. Its reported <status>
	// (completed/failed/killed) doesn't matter — only that one exists.
	BackgroundTaskResolved BackgroundTaskStatus = "resolved"
	// BackgroundTaskOutstandingFresh means no matching notification has
	// landed yet, and the marker is younger than the cap passed to
	// ReadBackgroundTasks.
	BackgroundTaskOutstandingFresh BackgroundTaskStatus = "outstanding-fresh"
	// BackgroundTaskOutstandingAgedOut means no matching notification has
	// landed, and the marker has exceeded the cap. Neutral: callers must not
	// treat this as evidence of anything, gating or disproving.
	BackgroundTaskOutstandingAgedOut BackgroundTaskStatus = "outstanding-aged-out"
	// BackgroundTaskUnreadable means the transcript file exists but every
	// non-blank line in it failed to parse as JSON.
	BackgroundTaskUnreadable BackgroundTaskStatus = "unreadable"
	// BackgroundTaskUnsupported means there is no transcript file at path at
	// all (e.g. a Codex session, which has no Claude-style transcript).
	BackgroundTaskUnsupported BackgroundTaskStatus = "unsupported"
)

// BackgroundTaskAgedOutCap is the cap gx's callers pass to
// ReadBackgroundTasks: how long an outstanding marker is trusted as evidence
// the agent isn't really finished. Past it, the background job is assumed
// abandoned or unresolvable rather than waited on forever.
const BackgroundTaskAgedOutCap = 2 * time.Hour

// BackgroundTaskMarker is one non-sidechain start marker (a backgrounded shell
// command, Monitor or async Agent) found in a transcript, together with its
// resolution state.
type BackgroundTaskMarker struct {
	TaskID    string
	Status    BackgroundTaskStatus
	StartedAt time.Time
	// Cap is how long the marker may stay outstanding before it ages out.
	Cap time.Duration
	// Kind is BackgroundTaskShell, BackgroundTaskMonitor or BackgroundTaskAgent.
	Kind string
	// Label is the launching tool call's description, else the first
	// backgroundLabelLen characters of its command.
	Label string
}

// Kinds of a BackgroundTaskMarker.
const (
	BackgroundTaskShell   = "shell"
	BackgroundTaskMonitor = "monitor"
	BackgroundTaskAgent   = "agent"
)

const backgroundLabelLen = 60

// toolUseInput is the part of a tool_use block that names what it launched.
type toolUseInput struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

func (in toolUseInput) label() string {
	if in.Description != "" {
		return in.Description
	}
	return truncateRunes(in.Command, backgroundLabelLen)
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// BackgroundTaskReading is ReadBackgroundTasks' result. FileStatus is set to
// BackgroundTaskUnreadable or BackgroundTaskUnsupported (with Markers left
// nil) when the read failed at the whole-file level; it is the zero value
// ("") otherwise, and callers should inspect Markers instead — including
// when Markers is empty, which just means no background task was found.
type BackgroundTaskReading struct {
	Markers    []BackgroundTaskMarker
	FileStatus BackgroundTaskStatus
	// AwaitingTurnEnd is true when the latest line resolving a marker has no
	// assistant turn end after it yet. The resolving notification wakes an
	// agent that ended its turn to wait, and the pane can still read idle
	// while that new turn runs short tool calls. Past cap it reads false.
	AwaitingTurnEnd bool
	// ValidUntil is the last instant at which this reading still holds for an
	// unchanged file: after it a marker or AwaitingTurnEnd ages out. Zero means
	// time cannot change the reading.
	ValidUntil time.Time
}

var backgroundKinds = map[string]string{
	TaskKindShell:    BackgroundTaskShell,
	TaskKindMonitor:  BackgroundTaskMonitor,
	TaskKindSubagent: BackgroundTaskAgent,
}

// ReadBackgroundTasks is a view over ReadTaskRecords: every non-sidechain
// background launch becomes a marker, resolved once a CLI report line names
// its task id. The records carry the parse facts; this view applies the cap.
//
// now is compared against each marker's own timestamp to decide
// outstanding-fresh vs outstanding-aged-out against cap — passed explicitly
// (rather than time.Now()) so callers and tests get a deterministic read. A
// Monitor expires on its own timeout, so its cap is that timeout plus a minute
// for its notice, when that is shorter than cap.
func ReadBackgroundTasks(path string, cap time.Duration, now time.Time) (BackgroundTaskReading, error) {
	scan, err := scanTaskRecords(path, false)
	if err != nil {
		return BackgroundTaskReading{}, err
	}
	if scan.missing {
		return BackgroundTaskReading{FileStatus: BackgroundTaskUnsupported}, nil
	}
	if scan.nonBlank > 0 && scan.parsed == 0 {
		return BackgroundTaskReading{FileStatus: BackgroundTaskUnreadable}, nil
	}

	var reading BackgroundTaskReading
	var lastReportedAt time.Time
	awaitingTurnEnd := false
	for _, r := range scan.records {
		if r.TaskID == "" {
			continue
		}
		if r.Reported {
			awaitingTurnEnd = awaitingTurnEnd || r.AwaitingTurnEnd
			if r.ReportedAt.After(lastReportedAt) {
				lastReportedAt = r.ReportedAt
			}
		}
	}
	reading.AwaitingTurnEnd = awaitingTurnEnd && now.Sub(lastReportedAt) <= cap
	expireAt := func(t time.Time) {
		if reading.ValidUntil.IsZero() || t.Before(reading.ValidUntil) {
			reading.ValidUntil = t
		}
	}
	if reading.AwaitingTurnEnd {
		expireAt(lastReportedAt.Add(cap))
	}
	for _, r := range scan.records {
		if r.TaskID == "" {
			continue
		}
		markerCap := cap
		if r.Kind == TaskKindMonitor {
			markerCap = min(cap, r.Timeout+time.Minute)
		}
		status := BackgroundTaskOutstandingFresh
		switch {
		case r.Reported:
			status = BackgroundTaskResolved
		case now.Sub(r.StartedAt) > markerCap:
			status = BackgroundTaskOutstandingAgedOut
		default:
			expireAt(r.StartedAt.Add(markerCap))
		}
		reading.Markers = append(reading.Markers, BackgroundTaskMarker{
			TaskID:    r.TaskID,
			Status:    status,
			StartedAt: r.StartedAt,
			Cap:       markerCap,
			Kind:      backgroundKinds[r.Kind],
			Label:     r.Label,
		})
	}
	return reading, nil
}
