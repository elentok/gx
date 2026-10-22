package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func logTicketPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "04-thing.md")
	writeFile(t, path, "---\nid: \"04\"\nstatus: claimed\ntype: implement\n---\n")
	return path
}

func question(by string) LogEvent {
	return LogEvent{Type: EventPauseQuestion, By: by, PauseKind: "question", Summary: "which?", Text: "which one?", Handoff: "h"}
}

func TestReadLog_MissingLogIsEmpty(t *testing.T) {
	events, err := ReadLog(logTicketPath(t))
	if err != nil || len(events) != 0 {
		t.Fatalf("ReadLog = %v, %v; want empty", events, err)
	}
}

func TestReadLog_SkipsTornLastLine(t *testing.T) {
	path := logTicketPath(t)
	if _, err := AppendLog(path, question("agent")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(LogPath(path), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"seq":2,"ts":"2026-01`)
	f.Close()

	events, err := ReadLog(path)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	// The next append cuts the torn tail off and continues the sequence.
	ev, err := AppendLog(path, LogEvent{Type: EventPauseAnswer, Question: 1, Source: "cli", Text: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Seq != 2 {
		t.Errorf("seq = %d, want 2", ev.Seq)
	}
	if events, err = ReadLog(path); err != nil || len(events) != 2 {
		t.Fatalf("ReadLog after append = %d events, %v", len(events), err)
	}
}

// Read shim: a log written before the rename keeps its old event types and
// kind field, and reads back in the current spelling.
func TestReadLog_FoldsOldEventTypesAndKind(t *testing.T) {
	path := logTicketPath(t)
	writeFile(t, LogPath(path), `{"seq":1,"ts":"2026-01-01T00:00:00Z","type":"park.question","by":"gx","park_kind":"zero-commit","summary":"s"}`+"\n"+
		`{"seq":2,"ts":"2026-01-01T00:00:01Z","type":"park.answer","question":1}`+"\n")

	events, err := ReadLog(path)
	if err != nil || len(events) != 2 {
		t.Fatalf("ReadLog = %v, %v; want 2 events", events, err)
	}
	if events[0].Type != EventPauseQuestion || events[0].PauseKind != "zero-commit" || events[0].LegacyKind != "" {
		t.Errorf("question = %+v, want pause.question with kind zero-commit", events[0])
	}
	if events[1].Type != EventPauseAnswer {
		t.Errorf("answer type = %q, want %q", events[1].Type, EventPauseAnswer)
	}
	if q := OpenQuestion(events); q != nil {
		t.Errorf("OpenQuestion = %+v, want none: the old answer closes the old question", q)
	}
}

func TestReadLog_MalformedMiddleLineIsError(t *testing.T) {
	path := logTicketPath(t)
	good := `{"seq":1,"ts":"2026-01-01T00:00:00Z","type":"park.closed","question":1}` + "\n"
	writeFile(t, LogPath(path), good+"not json\n"+good)

	_, err := ReadLog(path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want one naming line 2", err)
	}
}

func TestAppendLog_SeqMonotonicUnderConcurrentAppends(t *testing.T) {
	path := logTicketPath(t)
	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if _, err := AppendLog(path, LogEvent{Type: EventPauseAnswer, Question: 1, Source: "cli"}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	events, err := ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != n {
		t.Fatalf("got %d events, want %d", len(events), n)
	}
	for i, ev := range events {
		if ev.Seq != i+1 {
			t.Fatalf("event %d has seq %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestAppendLog_RefusesOverlongText(t *testing.T) {
	path := logTicketPath(t)
	ev := question("agent")
	ev.Text = strings.Repeat("x", 4001)
	if _, err := AppendLog(path, ev); err == nil {
		t.Fatal("4,001-character text accepted")
	}
	ev.Text = strings.Repeat("é", 4000)
	if _, err := AppendLog(path, ev); err != nil {
		t.Fatalf("4,000-character text refused: %v", err)
	}
	if _, err := os.Stat(LogPath(path)); err != nil {
		t.Fatal(err)
	}
}

func TestAppendLog_AgentQuestionNeedsHandoff(t *testing.T) {
	ev := question("agent")
	ev.Handoff = ""
	if _, err := AppendLog(logTicketPath(t), ev); err == nil {
		t.Fatal("agent question without handoff accepted")
	}
	ev.By = "gx"
	if _, err := AppendLog(logTicketPath(t), ev); err != nil {
		t.Fatalf("gx question refused: %v", err)
	}
}

func TestOpenQuestionAndRounds(t *testing.T) {
	mk := func(seq int, typ string, q int) LogEvent { return LogEvent{Seq: seq, Type: typ, Question: q} }
	events := []LogEvent{
		mk(1, EventPauseQuestion, 0),
		mk(2, EventPausePermission, 0),
		mk(3, EventPauseAnswer, 1),
		mk(4, EventPauseQuestion, 0),
		mk(5, EventPauseClosed, 4),
		mk(6, EventPauseQuestion, 0),
		mk(7, EventPausePermission, 0),
	}

	rounds := Rounds(events)
	if len(rounds) != 3 {
		t.Fatalf("got %d rounds, want 3", len(rounds))
	}
	if rounds[0].End == nil || rounds[0].End.Seq != 3 {
		t.Errorf("round 1 end = %v, want seq 3", rounds[0].End)
	}
	if rounds[1].End == nil || rounds[1].End.Type != EventPauseClosed {
		t.Errorf("round 2 end = %v, want pause.closed", rounds[1].End)
	}
	if rounds[2].End != nil {
		t.Errorf("round 3 end = %v, want open", rounds[2].End)
	}

	if q := OpenQuestion(events); q == nil || q.Seq != 6 {
		t.Errorf("OpenQuestion = %v, want seq 6", q)
	}
	if q := OpenQuestion(events[:3]); q != nil {
		t.Errorf("OpenQuestion of an answered log = %v, want nil", q)
	}
	if q := OpenQuestion(events[:5]); q != nil {
		t.Errorf("OpenQuestion of a closed log = %v, want nil", q)
	}
	if q := OpenQuestion(nil); q != nil {
		t.Errorf("OpenQuestion(nil) = %v, want nil", q)
	}
}

func TestLoad_IgnoresLogFiles(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "my-epic", "issues", "01-first.md")
	writeFile(t, md, "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n")
	if _, err := AppendLog(md, question("agent")); err != nil {
		t.Fatal(err)
	}

	epics, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 1 || len(epics[0].Tickets) != 1 {
		t.Fatalf("got %d epics, tickets %v; want 1 ticket", len(epics), epics)
	}
}
