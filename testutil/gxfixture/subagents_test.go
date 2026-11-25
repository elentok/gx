package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// subChat reads the chat of the session the fixture printed.
func (r *running) subChat(t *testing.T) wire.SessionChatResponse {
	t.Helper()
	if r.Subagents == nil {
		t.Fatal("output has no subagents")
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", r.Subagents.SessionID, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	return chat
}

func readMeta(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSubagents_Shapes(t *testing.T) {
	t.Parallel()
	type shape struct {
		name  string
		calls []string // names in Output.Subagents.Calls, with the description its tools entry shows
		check func(t *testing.T, r *running, chat wire.SessionChatResponse)
	}
	descs := map[string]string{"running": bgtx.SubRunningDesc, "finished": bgtx.SubFinishedDesc, "missing": bgtx.SubMissingDesc}
	checkRunning := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		var row *wire.SessionTask
		for i := range chat.Tasks {
			if chat.Tasks[i].Label == bgtx.SubRunningDesc {
				row = &chat.Tasks[i]
			}
		}
		// The current server links a subagent's file through the call's async_launched
		// result, which a foreground call lacks, so the row has no model, tokens or tool
		// count yet; the file itself holds them for the later web tickets.
		if row == nil || row.Finished {
			t.Fatalf("running subagent row = %+v in %+v, want a running row", row, chat.Tasks)
		}
		raw, err := os.ReadFile(r.TaskFiles[bgtx.SubRunningFileKey])
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"isSidechain":true`, `"model":"` + "claude-sonnet-5-5", `"usage"`, `"name":"Read"`} {
			if !strings.Contains(string(raw), want) {
				t.Fatalf("running subagent file lacks %s:\n%s", want, raw)
			}
		}
		meta := readMeta(t, r.TaskFiles[bgtx.SubRunningMetaKey])
		if meta["toolUseId"] != bgtx.SubRunningCall {
			t.Fatalf("running meta = %v, want toolUseId %s", meta, bgtx.SubRunningCall)
		}
		if _, ok := meta["finished"]; ok {
			t.Fatalf("running meta = %v, want no finished", meta)
		}
	}
	finished := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		meta := readMeta(t, r.TaskFiles[bgtx.SubFinishedMetaKey])
		if meta["toolUseId"] != bgtx.SubFinishedCall || meta["finished"] != true {
			t.Fatalf("finished meta = %v, want toolUseId %s and finished", meta, bgtx.SubFinishedCall)
		}
		raw, err := os.ReadFile(r.TaskFiles[bgtx.SubFinishedFileKey])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"name":"SubagentHandback"`) || !strings.Contains(string(raw), bgtx.SubHandback) {
			t.Fatalf("finished file has no handback:\n%s", raw)
		}
	}
	none := func(*testing.T, *running, wire.SessionChatResponse) {}
	cases := []shape{
		{"claude", []string{"running", "finished", "missing"}, func(t *testing.T, r *running, c wire.SessionChatResponse) {
			checkRunning(t, r, c)
			finished(t, r, c)
		}},
		{"foreground-running", []string{"running"}, checkRunning},
		{"finished-handback", []string{"finished"}, finished},
		{"file-gone", []string{"missing"}, none},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "subagents", nil, "--shape", tc.name)
			chat := r.subChat(t)
			for _, name := range tc.calls {
				if r.Subagents.Calls[name] == "" {
					t.Fatalf("output calls = %v, want %q", r.Subagents.Calls, name)
				}
				found := false
				for _, m := range chat.Messages {
					for _, c := range m.Calls {
						found = found || (m.Kind == "tools" && strings.Contains(c, descs[name]))
					}
				}
				if !found {
					t.Fatalf("no tools entry names %q (call %s): %+v", descs[name], r.Subagents.Calls[name], chat.Messages)
				}
			}
			tc.check(t, r, chat)
		})
	}
}

func TestSubagents_BadShapeFails(t *testing.T) {
	t.Parallel()
	want := "known: " + strings.Join(spec.SubagentsShapes, ", ")
	out, err := exec.Command(fixturetest.FixtureBin, "--gx", fixturetest.GxBin, "--fakeclaude", fixturetest.FakeBin,
		"--fakegh", fixturetest.FakeGHBin, "--shape", "bogus", "subagents").CombinedOutput()
	if err == nil || !strings.Contains(string(out), want) {
		t.Fatalf("--shape bogus: err = %v, output = %q, want %q", err, out, want)
	}
}
