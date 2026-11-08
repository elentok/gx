package backgroundtask_test

import (
	"errors"
	"testing"

	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/transcript"
)

func TestResolve(t *testing.T) {
	fresh := transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{{TaskID: "t1", Status: transcript.BackgroundTaskOutstandingFresh}}}
	clean := transcript.BackgroundTaskReading{Markers: []transcript.BackgroundTaskMarker{{TaskID: "t1", Status: transcript.BackgroundTaskResolved}}}
	awaiting := transcript.BackgroundTaskReading{AwaitingTurnEnd: true}
	missing := transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnsupported}
	tests := []struct {
		name     string
		pid      backgroundtask.Pid
		tr       transcript.BackgroundTaskReading
		err      error
		live     bool
		source   backgroundtask.Source
		disagree bool
	}{
		{"shell and outstanding task", backgroundtask.PidShell, fresh, nil, true, backgroundtask.SourceBoth, false},
		{"idle and outstanding task", backgroundtask.PidNoShell, fresh, nil, true, backgroundtask.SourceTranscript, false},
		{"no pid file and outstanding task", backgroundtask.PidUnknown, fresh, nil, true, backgroundtask.SourceTranscript, false},
		{"idle and awaiting turn end", backgroundtask.PidNoShell, awaiting, nil, true, backgroundtask.SourceTranscript, false},
		{"shell and clean transcript is stale", backgroundtask.PidShell, clean, nil, false, backgroundtask.SourceNone, true},
		{"shell and missing transcript stands", backgroundtask.PidShell, missing, nil, true, backgroundtask.SourcePidFile, false},
		{"shell and read error stands", backgroundtask.PidShell, transcript.BackgroundTaskReading{}, errors.New("boom"), true, backgroundtask.SourcePidFile, false},
		{"idle and read error is not live", backgroundtask.PidNoShell, fresh, errors.New("boom"), false, backgroundtask.SourceNone, false},
		{"idle and clean transcript", backgroundtask.PidNoShell, clean, nil, false, backgroundtask.SourceNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backgroundtask.Resolve(tt.pid, tt.tr, tt.err)
			if got.Live != tt.live || got.Source != tt.source || got.Disagree != tt.disagree {
				t.Fatalf("Resolve = live %v source %q disagree %v, want %v %q %v", got.Live, got.Source, got.Disagree, tt.live, tt.source, tt.disagree)
			}
		})
	}
}
