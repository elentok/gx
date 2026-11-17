package brief

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

func lastIndexEntry(t *testing.T) brief.IndexEntry {
	t.Helper()
	data := cmdtest.EnvValue(cmdtest.EnvFor(t), "XDG_DATA_HOME")
	entries, err := brief.ReadIndex(filepath.Join(data, "gx"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("ReadIndex = %v, %v", entries, err)
	}
	return entries[len(entries)-1]
}

func TestBrief_IndexAttemptCountsRunsOnOneTree(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n")

	for want := 1; want <= 2; want++ {
		if out, code := runBriefCmd(t, repo, "go-test", "./..."); code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if got := lastIndexEntry(t).Attempt; got != want {
			t.Errorf("attempt = %d, want %d", got, want)
		}
	}
}

func TestBrief_IndexOrigin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, value, want string }{
		{"release", "release", "release"},
		{"unset", "", "agent"},
		{"other", "ci", "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value != "" {
				cmdtest.SetEnvFor(t, brief.EnvCheckOrigin, tc.value)
			}
			if out, code := runBriefCmd(t, goModule(t), "go-test", "./..."); code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			if got := lastIndexEntry(t).Origin; got != tc.want {
				t.Errorf("origin = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBrief_IndexSignatureAndMachineSample(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestBad(t *testing.T) { t.Fatal(\"boom: "+strings.Repeat("x", 400)+"\") }\n")
	calls := 0
	samples := []brief.Sample{{Load: 1.5, Procs: 100}, {Load: 2.5, Procs: 120}}
	fake := func() brief.Sample { calls++; return samples[min(calls-1, 1)] }

	if _, code := runBriefSampled(t, fake, dir, "go-test", "./..."); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	e := lastIndexEntry(t)
	if e.LoadStart != 1.5 || e.LoadEnd != 2.5 || e.ProcsStart != 100 || e.ProcsEnd != 120 {
		t.Errorf("load/procs = %v %v %d %d", e.LoadStart, e.LoadEnd, e.ProcsStart, e.ProcsEnd)
	}
	if !strings.Contains(e.Signature, "boom") || len([]rune(e.Signature)) > 200 {
		t.Errorf("signature = %q", e.Signature)
	}
	raw, _ := json.Marshal(e)
	if !strings.Contains(string(raw), `"origin":"agent"`) {
		t.Errorf("line = %s", raw)
	}
}
