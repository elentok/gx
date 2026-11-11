package tuiclaude

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestAppendClassifierNoVerdict_EachTurnAddsOneErrorTheOutageCounterSees(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	for want := 1; want <= 2; want++ {
		if err := appendClassifierNoVerdict(path); err != nil {
			t.Fatal(err)
		}
		got, err := transcript.CountClassifierNoVerdict(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("after %d turns the counter read %d", want, got)
		}
	}
}
