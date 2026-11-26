package codexsession

import (
	"encoding/json"
	"fmt"
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
			got, err := ReadInFlightTools(path, false)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReadInFlightTools_ExecShape(t *testing.T) {
	t.Parallel()
	started := `{"type":"event_msg","payload":{"type":"task_started"}}`
	exec := func(id, input string) string {
		in, _ := json.Marshal(input)
		return `{"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":"` + id + `","input":` + string(in) + `}}`
	}
	out := func(id string, texts ...string) string {
		var items []map[string]string
		for _, s := range texts {
			items = append(items, map[string]string{"type": "input_text", "text": s})
		}
		b, _ := json.Marshal(items)
		s, _ := json.Marshal(string(b))
		return `{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"` + id + `","output":` + string(s) + `}}`
	}
	wait := func(id string, cell int) string {
		return fmt.Sprintf(`{"type":"response_item","payload":{"type":"function_call","name":"wait","call_id":%q,"arguments":"{\"cell_id\":%d}"}}`, id, cell)
	}
	waitOut := func(id, text string) string {
		return `{"type":"response_item","payload":{"type":"function_call_output","call_id":"` + id + `","output":"` + text + `"}}`
	}
	plan := `{"type":"response_item","payload":{"type":"function_call","name":"update_plan","call_id":"u","arguments":"{}"}}`
	run := `tools.exec_command({cmd:"make"})`
	label := func(l string) transcript.InFlightTool { return transcript.InFlightTool{Kind: "shell", Label: l} }

	for _, tc := range []struct {
		name  string
		lines []string
		want  []transcript.InFlightTool
	}{
		{"open exec shows its command", []string{started, exec("a", run)}, []transcript.InFlightTool{label("make")}},
		{"exec without a command stays exec", []string{started, exec("a", "await tools.write_stdin({session_id:1})")},
			[]transcript.InFlightTool{{Kind: "other", Label: "exec"}}},
		{"yielded cell stays, its wait adds nothing", []string{started, exec("a", run), out("a", "Script running with cell ID 3"), wait("w", 3)},
			[]transcript.InFlightTool{label("make")}},
		{"wait still running keeps it", []string{started, exec("a", run), out("a", "Script running with cell ID 3"), wait("w", 3), waitOut("w", "Script running with cell ID 3")},
			[]transcript.InFlightTool{label("make")}},
		{"wait completed ends it", []string{started, exec("a", run), out("a", "Script running with cell ID 3"), wait("w", 3), waitOut("w", "Script completed")},
			[]transcript.InFlightTool{}},
		{"session cell is a background process", []string{started, exec("a", run), out("a", "Script completed", `{"session_id":9}`)},
			[]transcript.InFlightTool{}},
		{"other calls keep their label", []string{started, exec("a", run), plan},
			[]transcript.InFlightTool{{Kind: "other", Label: "update_plan"}, label("make")}},
		{"wait of an unknown cell is labelled as before", []string{started, wait("w", 4)},
			[]transcript.InFlightTool{{Kind: "other", Label: "wait"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tc.lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := ReadInFlightTools(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReadInFlightTools_ItemCompletedEndsTheCommand(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	lines := []string{
		`{"timestamp":"` + at(0) + `","type":"event_msg","payload":{"type":"task_started"}}`,
		execLine(1, "a", `tools.exec_command({cmd:"make"})`),
		cellOut(2, "a", "Script running with cell ID 7"),
		execLine(3, "b", `tools.exec_command({cmd:"sleep 99"})`),
		commandItem(10, 2, "5", "make", 0, "ok"),
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInFlightTools(path, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []transcript.InFlightTool{{Kind: "shell", Label: "sleep 99"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadChildInFlightTools_OnlyTheChildsOwnCalls(t *testing.T) {
	t.Parallel()
	fn := func(ord int, id, cmd string) string {
		return fmt.Sprintf(`{"ordinal":%d,"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":%q,"arguments":"{\"cmd\":\"%s\"}"}}`, ord, id, cmd)
	}
	cell := func(ord int, id, cmd string) string {
		return fmt.Sprintf(`{"ordinal":%d,"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":%q,"input":"tools.exec_command({cmd:\"%s\"})"}}`, ord, id, cmd)
	}
	// A line without an ordinal before the start is inherited too.
	unnumbered := `{"type":"response_item","payload":{"type":"function_call","name":"update_plan","call_id":"u","arguments":"{}"}}`
	own := []transcript.InFlightTool{{Kind: "shell", Label: "own"}}

	for _, tc := range []struct {
		name      string
		execShape bool
		lines     []string
		want      []transcript.InFlightTool
	}{
		{"old shape", false, []string{unnumbered, fn(1, "p", "parent"), fn(3, "c", "own")}, own},
		{"old shape, only inherited calls", false, []string{unnumbered, fn(1, "p", "parent")}, []transcript.InFlightTool{}},
		{"exec cells", true, []string{unnumbered, cell(1, "p", "parent"), cell(3, "c", "own")}, own},
		{"exec cells, only inherited calls", true, []string{unnumbered, cell(1, "p", "parent")}, []transcript.InFlightTool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tc.lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := ReadChildInFlightTools(path, 3, tc.execShape)
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
	got, err := ReadInFlightTools(filepath.Join(t.TempDir(), "none.jsonl"), false)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v, want nothing", got, err)
	}
}
