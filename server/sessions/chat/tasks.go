package chat

import (
	"slices"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// taskState is one record after the end rules.
type taskState struct {
	rec        transcript.TaskRecord
	finished   bool
	at         time.Time // finish time; zero with noDuration
	outcome    string
	noDuration bool
	// groupEnd is when the task stops counting as running for grouping. A
	// no-duration row ends at: start+cap for age-out, RunStartedAt for an
	// earlier run, now for an exited session (its group is closed).
	groupEnd time.Time
}

// AssembleTasks turns parsed records into the open Task group: the latest
// group, only while a task of it runs, ordered by start. It is pure and runs
// on every read, so the clock-based rules move without a re-parse.
//
// Per record the first matching rule wins: matched end (or turn end for a
// foreground call), shell trailer, Monitor timeout, exited session or earlier
// run, age-out.
func AssembleTasks(records []transcript.TaskRecord, meta Meta, now time.Time) []wire.SessionTask {
	states := make([]taskState, len(records))
	for i, r := range records {
		states[i] = resolveTask(r, meta, now)
	}
	slices.SortStableFunc(states, func(a, b taskState) int { return a.rec.StartedAt.Compare(b.rec.StartedAt) })

	// A group starts when a task starts while nothing else of it runs.
	groupStart, ends, anyRunning := 0, time.Time{}, false
	for i, s := range states {
		if i == 0 || (!anyRunning && !s.rec.StartedAt.Before(ends)) {
			groupStart = i
		}
		if s.finished {
			ends = laterOf(ends, s.groupEnd)
		} else {
			anyRunning = true
		}
	}
	// Every earlier group has ended, so the latest is open only while a task of
	// it runs; a running task is never before groupStart's end.
	if !anyRunning {
		return nil
	}
	out := make([]wire.SessionTask, 0, len(states)-groupStart)
	for _, s := range states[groupStart:] {
		out = append(out, wireTask(s))
	}
	return out
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func resolveTask(r transcript.TaskRecord, meta Meta, now time.Time) taskState {
	s := taskState{rec: r}
	finish := func(at time.Time, outcome string) {
		s.finished, s.at, s.outcome, s.groupEnd = true, at, outcome, at
	}
	noDuration := func(groupEnd time.Time) {
		s.finished, s.noDuration, s.groupEnd = true, true, groupEnd
	}
	switch {
	case r.End != nil:
		finish(r.End.At, taskOutcome(r.End.Outcome))
	case (r.Kind == transcript.TaskKindCall || r.Kind == transcript.TaskKindSubagent) && !r.TurnEnd.IsZero():
		finish(r.TurnEnd, "")
	case r.Trailer != nil:
		finish(r.Trailer.At, taskOutcome(r.Trailer.Outcome))
	case r.Kind == transcript.TaskKindMonitor && r.Timeout > 0 && now.After(r.StartedAt.Add(r.Timeout)):
		finish(r.StartedAt.Add(r.Timeout), "")
	case meta.Status.State == wire.SessionExited:
		noDuration(now)
	case r.StartedAt.Before(meta.RunStartedAt):
		noDuration(meta.RunStartedAt)
	case now.Sub(r.StartedAt) > transcript.BackgroundTaskAgedOutCap:
		noDuration(r.StartedAt.Add(transcript.BackgroundTaskAgedOutCap))
	}
	return s
}

// taskOutcome maps a record's outcome to the wire's; unknown words mean none.
func taskOutcome(o string) string {
	switch o {
	case transcript.TaskOutcomeSuccess, transcript.TaskOutcomeFailure:
		return o
	case transcript.TaskOutcomeKilled, transcript.TaskOutcomeStopped:
		return transcript.TaskOutcomeKilled
	}
	return ""
}

func wireTask(s taskState) wire.SessionTask {
	r := s.rec
	t := wire.SessionTask{
		Kind: r.Kind, Label: r.Label, CallID: r.CallID, StartedAt: r.StartedAt.Format(time.RFC3339),
		Finished: s.finished, Outcome: s.outcome, NoDuration: s.noDuration,
		Command: r.Command, ExitCode: r.ExitCode, OutputBytes: r.OutputBytes, LastLine: r.LastLine,
		Model: r.Model, Tokens: r.Tokens, ToolCalls: r.ToolCalls, CurrentCall: r.CurrentCall,
		CurrentCallPath: r.CurrentCallPath, EventCount: r.EventCount,
	}
	if !s.at.IsZero() {
		t.FinishedAt = s.at.Format(time.RFC3339)
	}
	if !r.LastEventAt.IsZero() {
		t.LastEventAt = r.LastEventAt.Format(time.RFC3339)
	}
	return t
}
