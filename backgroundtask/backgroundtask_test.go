package backgroundtask_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func startLine(taskID string, at time.Time) string {
	return `{"isSidechain":false,"type":"user","timestamp":"` + at.Format(time.RFC3339Nano) + `","toolUseResult":{"backgroundTaskId":"` + taskID + `"}}` + "\n"
}

func newReader(t *testing.T, content string, now *time.Time) (*backgroundtask.Reader, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return &backgroundtask.Reader{Home: home, Now: func() time.Time { return *now }}, path
}

func TestReader_UnchangedTranscriptIsParsedOnce(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rd, path := newReader(t, startLine("t1", now), &now)
	if !rd.Read("sess", backgroundtask.PidUnknown).Live {
		t.Fatal("first read: want live")
	}
	// Same size and mtime, different content: only a re-parse would notice.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", int(fi.Size()))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if !rd.Read("sess", backgroundtask.PidUnknown).Live {
			t.Fatal("unchanged file was parsed again")
		}
	}
}

func TestReader_GrownTranscriptIsSeen(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rd, path := newReader(t, startLine("t1", now), &now)
	if got := len(rd.Read("sess", backgroundtask.PidUnknown).Tasks); got != 1 {
		t.Fatalf("tasks = %d, want 1", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(startLine("t2", now)); err != nil {
		t.Fatal(err)
	}
	if got := len(rd.Read("sess", backgroundtask.PidUnknown).Tasks); got != 2 {
		t.Fatalf("tasks after append = %d, want 2", got)
	}
}

func TestReader_AgedOutTaskIsSeenWithoutFileChange(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rd, _ := newReader(t, startLine("t1", now), &now)
	if !rd.Read("sess", backgroundtask.PidUnknown).Live {
		t.Fatal("fresh task: want live")
	}
	now = now.Add(transcript.BackgroundTaskAgedOutCap + time.Second)
	if rd.Read("sess", backgroundtask.PidUnknown).Live {
		t.Fatal("aged-out task still live from cache")
	}
}
