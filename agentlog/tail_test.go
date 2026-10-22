package agentlog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	textLine   = `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`
	resultLine = `{"type":"result","result":"done"}`
)

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func step(t *testing.T, tl *Tail) ([]string, bool) {
	t.Helper()
	lines, ended, err := tl.Step()
	if err != nil {
		t.Fatal(err)
	}
	return lines, ended
}

func TestTail_Steps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	appendFile(t, path, "skipped\n"+textLine+"\n")
	ended := false
	tl, err := OpenTail(Log{Path: path, Offset: int64(len("skipped\n")), Ended: func() bool { return ended }}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()

	if lines, done := step(t, tl); !slices.Equal(lines, []string{"[text] hi"}) || done {
		t.Fatalf("first step = %q, %v", lines, done)
	}
	appendFile(t, path, resultLine[:10])
	if lines, done := step(t, tl); len(lines) != 0 || done {
		t.Fatalf("partial line step = %q, %v", lines, done)
	}
	appendFile(t, path, resultLine[10:]+"\n")
	if lines, done := step(t, tl); !slices.Equal(lines, []string{"[result] done"}) || done {
		t.Fatalf("completed line step = %q, %v", lines, done)
	}

	// Written just before the agent exited: the last poll still reads it.
	appendFile(t, path, textLine+"\n")
	ended = true
	if lines, done := step(t, tl); !slices.Equal(lines, []string{"[text] hi"}) || !done {
		t.Fatalf("last poll =%q, %v", lines, done)
	}
	if lines, done := step(t, tl); len(lines) != 0 || !done {
		t.Fatalf("after end = %q, %v", lines, done)
	}
}

func TestTail_FlushRendersPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	appendFile(t, path, resultLine)
	tl, err := OpenTail(Log{Path: path}, func(raw []byte) []string { return []string{string(raw)} })
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	if lines, done := step(t, tl); len(lines) != 0 || done {
		t.Fatalf("step = %q, %v", lines, done)
	}
	if got := tl.Flush(); !slices.Equal(got, []string{resultLine}) {
		t.Fatalf("Flush = %q", got)
	}
}
