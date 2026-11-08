package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		{"Read", `{"file_path":"/a"}`, InFlightTool{Kind: "other", Label: "Read"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assertInFlight(t, readInFlight(t, ifPrompt, ifUse("a", c.name, c.input, false)), []InFlightTool{c.want})
		})
	}
}

func TestReadInFlightTools_LabelIsFirstLineCapped(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 80)
	got := readInFlight(t, ifPrompt, ifUse("a", "Bash", fmt.Sprintf(`{"command":%q}`, long+"\nsecond"), false))
	assertInFlight(t, got, []InFlightTool{{Kind: "shell", Label: strings.Repeat("x", 60) + "…"}})
}
