package nativerunner

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
)

// A prompt sent mid-turn is queued; the turn's result must not read as
// finished, because claude starts the queued prompt next on its own.
func TestTracker_QueuedCommandKeepsWorking(t *testing.T) {
	var tr tracker
	steps := []struct {
		line string
		want agentrunner.State
		turn int
	}{
		{`{"type":"command_lifecycle","command_uuid":"a","state":"queued"}`, agentrunner.StateWorking, 0},
		{`{"type":"command_lifecycle","command_uuid":"a","state":"started"}`, agentrunner.StateWorking, 1},
		{`{"type":"system","subtype":"session_state_changed","state":"running"}`, agentrunner.StateWorking, 1},
		{`{"type":"command_lifecycle","command_uuid":"b","state":"queued"}`, agentrunner.StateWorking, 1},
		{`{"type":"result","subtype":"success"}`, agentrunner.StateWorking, 1},
		{`{"type":"command_lifecycle","command_uuid":"a","state":"completed"}`, agentrunner.StateWorking, 1},
		{`{"type":"command_lifecycle","command_uuid":"b","state":"started"}`, agentrunner.StateWorking, 2},
		{`{"type":"result","subtype":"success"}`, agentrunner.StateIdle, 2},
	}
	for i, step := range steps {
		e, ok := parseEvent([]byte(step.line))
		if !ok {
			t.Fatalf("step %d: unparsable %s", i, step.line)
		}
		tr.apply(e)
		if st := tr.status(); st.State != step.want || st.Turn != step.turn {
			t.Fatalf("step %d (%s): status = %+v, want %s turn %d", i, step.line, st, step.want, step.turn)
		}
	}
}

// A second queued command that claude starts inside a still-running turn is
// a new turn of its own.
func TestTracker_CommandStartedMidTurnCountsAsTurn(t *testing.T) {
	var tr tracker
	for _, line := range []string{
		`{"type":"command_lifecycle","command_uuid":"a","state":"started"}`,
		`{"type":"assistant"}`,
		`{"type":"command_lifecycle","command_uuid":"b","state":"started"}`,
	} {
		e, _ := parseEvent([]byte(line))
		tr.apply(e)
	}
	if st := tr.status(); st.Turn != 2 || !tr.started("b") {
		t.Fatalf("status = %+v, want turn 2 with b started", st)
	}
}
