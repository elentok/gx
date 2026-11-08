package codexsession

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestReadInFlightTools(t *testing.T) {
	t.Parallel()
	started := `{"type":"event_msg","payload":{"type":"task_started"}}`
	call := func(typ, name, id, args string) string {
		return `{"type":"response_item","payload":{"type":"` + typ + `","name":"` + name + `","call_id":"` + id + `",` + args + `}}`
	}
	exec := func(id, cmd string) string {
		return call("function_call", "exec_command", id, `"arguments":"{\"cmd\":\"`+cmd+`\"}"`)
	}
	output := func(typ, id string) string {
		return `{"type":"response_item","payload":{"type":"` + typ + `","call_id":"` + id + `","output":"ok"}}`
	}
	long := strings.Repeat("x", 80)

	for _, tc := range []struct {
		name  string
		lines []string
		want  []transcript.InFlightTool
	}{
		{"empty", nil, []transcript.InFlightTool{}},
		{"unmatched call, newest first", []string{started, exec("a", "one"), exec("b", "two")},
			[]transcript.InFlightTool{{Kind: "shell", Label: "two"}, {Kind: "shell", Label: "one"}}},
		{"matched call", []string{started, exec("a", "one"), output("function_call_output", "a")}, []transcript.InFlightTool{}},
		{"earlier turn stays hidden", []string{started, exec("a", "old"), started, exec("b", "new")},
			[]transcript.InFlightTool{{Kind: "shell", Label: "new"}}},
		{"custom tool call", []string{started,
			call("custom_tool_call", "apply_patch", "p", `"input":"*** Begin Patch\n*** Add File: x"`),
			call("custom_tool_call", "apply_patch", "q", `"input":"*** Begin Patch"`),
			output("custom_tool_call_output", "q")},
			[]transcript.InFlightTool{{Kind: "other", Label: "apply_patch"}}},
		{"web search", []string{started, call("function_call", "web_search", "w", `"arguments":"{\"query\":\"go slices\"}"`)},
			[]transcript.InFlightTool{{Kind: "search", Label: "go slices"}}},
		{"unknown tool is other", []string{started, call("function_call", "update_plan", "u", `"arguments":"{}"`)},
			[]transcript.InFlightTool{{Kind: "other", Label: "update_plan"}}},
		{"label capped", []string{started, exec("a", long)},
			[]transcript.InFlightTool{{Kind: "shell", Label: long[:60] + "…"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tc.lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := ReadInFlightTools(path)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReadInFlightTools_MissingFile(t *testing.T) {
	t.Parallel()
	got, err := ReadInFlightTools(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v, want nothing", got, err)
	}
}
