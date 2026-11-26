package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func ifUse(id, name, input string, sidechain bool) string {
	return fmt.Sprintf(`{"type":"assistant","isSidechain":%t,"message":{"content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, sidechain, id, name, input)
}

func ifResult(id string) string {
	return fmt.Sprintf(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}}`, id)
}

const ifPrompt = `{"type":"user","message":{"content":"go"}}`

func readInFlight(t *testing.T, lines ...string) []InFlightTool {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInFlightTools(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertInFlight(t *testing.T, got, want []InFlightTool) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("in flight = %v, want %v", got, want)
	}
}

// An image record is one line of megabytes of base64; the readers behind the
// chat Loader must still read the transcript around it.
func TestLoaderReaders_LineOverTenMegabytes(t *testing.T) {
	t.Parallel()
	huge := `{"type":"user","timestamp":"2026-01-02T03:04:05Z","message":{"content":"` + strings.Repeat("x", 11*1024*1024) + `"}}`
	path := filepath.Join(t.TempDir(), "t.jsonl")
	body := strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-02T03:04:05Z","message":{"content":"go"}}`,
		huge,
		ifUse("a", "Bash", `{"command":"one"}`, false),
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := ReadInFlightTools(path); err != nil {
		t.Errorf("ReadInFlightTools: %v", err)
	} else {
		assertInFlight(t, got, []InFlightTool{{Kind: "shell", Label: "one"}})
	}
	if _, err := ReadBackgroundTasks(path, time.Hour, time.Now()); err != nil {
		t.Errorf("ReadBackgroundTasks: %v", err)
	}
	if lines, ok, err := ReadAll(path); err != nil || !ok || len(lines) == 0 {
		t.Errorf("ReadAll = %d lines, ok=%v, err=%v", len(lines), ok, err)
	}
	if _, ok, err := FirstLineTimestamp(path); err != nil || !ok {
		t.Errorf("FirstLineTimestamp ok=%v, err=%v", ok, err)
	}
}

func TestReadInFlightTools_NewestFirst(t *testing.T) {
	t.Parallel()
	got := readInFlight(t, ifPrompt,
		ifUse("a", "Bash", `{"command":"one"}`, false),
		ifUse("b", "Bash", `{"command":"two"}`, false),
		ifUse("c", "Bash", `{"command":"three"}`, false))
	assertInFlight(t, got, []InFlightTool{
		{Kind: "shell", Label: "three"}, {Kind: "shell", Label: "two"}, {Kind: "shell", Label: "one"},
	})
}

func TestReadInFlightTools_Excluded(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"matched":      {ifPrompt, ifUse("a", "Bash", `{"command":"x"}`, false), ifResult("a")},
		"earlier":      {ifPrompt, ifUse("a", "Bash", `{"command":"x"}`, false), ifPrompt},
		"sidechain":    {ifPrompt, ifUse("a", "Bash", `{"command":"x"}`, true)},
		"backgrounded": {ifPrompt, ifUse("a", "Bash", `{"command":"x","run_in_background":true}`, false), ifResult("a")},
		"nothing":      {ifPrompt},
	}
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := readInFlight(t, lines...); len(got) != 0 {
				t.Errorf("in flight = %v, want none", got)
			}
		})
	}
}

func TestReadInFlightTools_MatchedOneLeavesTheOther(t *testing.T) {
	t.Parallel()
	got := readInFlight(t, ifPrompt,
		ifUse("a", "Bash", `{"command":"one"}`, false),
		ifUse("b", "Bash", `{"command":"two"}`, false),
		ifResult("a"))
	assertInFlight(t, got, []InFlightTool{{Kind: "shell", Label: "two"}})
}

func TestReadInFlightTools_Kinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input string
		want        InFlightTool
	}{
		{"Bash", `{"command":"make test"}`, InFlightTool{Kind: "shell", Label: "make test"}},
		{"Agent", `{"description":"survey","subagent_type":"Explore"}`, InFlightTool{Kind: "agent", Label: "survey"}},
		{"Task", `{"subagent_type":"Explore"}`, InFlightTool{Kind: "agent", Label: "Explore"}},
		{"Grep", `{"pattern":"foo"}`, InFlightTool{Kind: "search", Label: "foo"}},
		{"Glob", `{"pattern":"*.go"}`, InFlightTool{Kind: "search", Label: "*.go"}},
		{"WebSearch", `{"query":"go generics"}`, InFlightTool{Kind: "search", Label: "go generics"}},
		{"WebFetch", `{"url":"https://x.dev"}`, InFlightTool{Kind: "search", Label: "https://x.dev"}},
		{"Read", `{"file_path":"/a"}`, InFlightTool{Kind: "other", Label: "Read /a", File: FileCall{Tool: "Read", Path: "/a"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertInFlight(t, readInFlight(t, ifPrompt, ifUse("a", c.name, c.input, false)), []InFlightTool{c.want})
		})
	}
}

func TestInFlightTool_LabelIn(t *testing.T) {
	t.Parallel()
	const dir = "/work/proj"
	// "Read " leaves 55 of the 60 runes to the path.
	fits := "/" + strings.Repeat("é", 54)
	name := strings.Repeat("n", 70) + ".go"
	cases := []struct {
		name, tool, filePath, label, path string
	}{
		{"relative inside the directory", "Read", dir + "/chat/tasks.go", "Read chat/tasks.go", "chat/tasks.go"},
		{"outside as written", "Read", "/etc/hosts", "Read /etc/hosts", "/etc/hosts"},
		{"already relative", "Read", "./chat/../tasks.go", "Read ./chat/../tasks.go", "./chat/../tasks.go"},
		{"sibling sharing the prefix", "Read", dir + "-two/a.go", "Read /work/proj-two/a.go", "/work/proj-two/a.go"},
		{"the directory itself", "Read", dir, "Read " + dir, dir},
		{"60 runes stay whole", "Read", fits, "Read " + fits, fits},
		{"folder cut", "Read", "/srv/" + strings.Repeat("a", 40) + "/server/chat/tasks.go", "Read …/server/chat/tasks.go", "…/server/chat/tasks.go"},
		{"folder cut keeps what fits", "Read", "/dropped/" + strings.Repeat("b", 42) + "/tasks.go", "Read …/" + strings.Repeat("b", 42) + "/tasks.go", "…/" + strings.Repeat("b", 42) + "/tasks.go"},
		{"file name cut", "Read", "/srv/" + name, "Read …" + name[len(name)-54:], "…" + name[len(name)-54:]},
		{"edit", "Edit", dir + "/a.go", "Edit a.go", "a.go"},
		{"write", "Write", dir + "/a.go", "Write a.go", "a.go"},
		{"first line only", "Read", "/a\nsecond", "Read /a", "/a"},
		{"missing file_path", "Read", "", "Read", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := map[string]any{}
			if c.filePath != "" {
				in["file_path"] = c.filePath
			}
			label, path := NewInFlightTool(c.tool, in).LabelIn(dir)
			if label != c.label || path != c.path {
				t.Errorf("LabelIn = %q, path %q; want %q, %q", label, path, c.label, c.path)
			}
			if n := utf8.RuneCountInString(label); n > 60 {
				t.Errorf("label is %d runes", n)
			}
		})
	}
}

func TestReadInFlightTools_LabelIsFirstLineCapped(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 80)
	got := readInFlight(t, ifPrompt, ifUse("a", "Bash", fmt.Sprintf(`{"command":%q}`, long+"\nsecond"), false))
	assertInFlight(t, got, []InFlightTool{{Kind: "shell", Label: strings.Repeat("x", 60) + "…"}})
}
