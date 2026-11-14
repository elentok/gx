package serverapi

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestServerDiff_VerbsHaveTheirFlags(t *testing.T) {
	for verb, flags := range map[string][]string{
		"diff-files":   {"json", "project", "commits"},
		"diff-patch":   {"json", "project", "commits"},
		"diff-commits": {"json", "project"},
		"diff-blob":    {"json", "project"},
	} {
		c, _, err := serverParent(Deps{}).Find([]string{verb})
		if err != nil || c.RunE == nil {
			t.Fatalf("server %s = %v, err %v; want a runnable verb", verb, c, err)
		}
		for _, f := range flags {
			if c.Flags().Lookup(f) == nil {
				t.Errorf("server %s has no --%s", verb, f)
			}
		}
	}
}

func TestServerDiff_RefAfterDashDashIsAnArgumentNotAFlag(t *testing.T) {
	c, _, err := serverParent(Deps{}).Find([]string{"diff-files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ParseFlags([]string{"--project", "p", "--", "--output=x", "main"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if got := c.Flags().Args(); !slices.Equal(got, []string{"--output=x", "main"}) {
		t.Errorf("args = %v, want the ref passed through to the server", got)
	}
}

func TestServerDiff_FilesTextListsRenamesAndMarksTruncation(t *testing.T) {
	var buf bytes.Buffer
	f := wire.DiffFiles{
		DiffRange: wire.DiffRange{MergeBase: "mb", SecondSHA: "tip"},
		Files: []wire.DiffFile{
			{Path: "new.txt", OldPath: "old.txt", Status: "R"},
			{Path: "a.txt", Status: "A", Added: 3},
		},
		Truncated: true,
	}
	if err := printDiffFiles(&buf, f); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"old.txt -> new.txt", "a.txt", "mb", "tip", "truncated"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output lacks %q: %s", want, buf.String())
		}
	}
}

func TestServerDiff_CommitsTextPrintsTheTrailerAddress(t *testing.T) {
	var buf bytes.Buffer
	c := wire.DiffCommits{Commits: []wire.DiffCommit{
		{SHA: "abc", Subject: "one", Ticket: "proj:epic/01"},
		{SHA: "def", Subject: "two"},
	}}
	if err := printDiffCommits(&buf, c); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "proj:epic/01\tabc") || !strings.HasPrefix(lines[1], "-\tdef") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestServerDiff_UnavailableCommitsAreTheirOwnNotice(t *testing.T) {
	r := wire.DiffRange{CommitsUnavailable: true}
	for name, print := range map[string]func(*bytes.Buffer) error{
		"files":   func(b *bytes.Buffer) error { return printDiffFiles(b, wire.DiffFiles{DiffRange: r}) },
		"patch":   func(b *bytes.Buffer) error { return printDiffPatch(b, wire.DiffPatch{DiffRange: r}) },
		"commits": func(b *bytes.Buffer) error { return printDiffCommits(b, wire.DiffCommits{DiffRange: r}) },
		"blob":    func(b *bytes.Buffer) error { return printDiffBlob(b, wire.DiffBlobs{CommitsUnavailable: true}) },
	} {
		var buf bytes.Buffer
		if err := print(&buf); err != nil || !strings.Contains(buf.String(), "commits not available") {
			t.Errorf("%s output = %q, err %v", name, buf.String(), err)
		}
	}
}
