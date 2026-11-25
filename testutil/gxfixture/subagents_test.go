package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	descs := map[string]string{"running": bgtx.SubRunningDesc, "finished": bgtx.SubFinishedDesc, "missing": bgtx.SubMissingDesc,
		"stopped": bgtx.SubStoppedDesc, "background": bgtx.SubBackgroundDesc, "nested": bgtx.SubNestedDesc, "starting": bgtx.SubStartingDesc, "marked": bgtx.SubMarkedDesc}
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
	// metaOf reads a seeded meta.json and checks its toolUseId is the call id the output printed.
	metaOf := func(t *testing.T, r *running, key, call string, finished bool) map[string]any {
		t.Helper()
		m := readMeta(t, r.TaskFiles[key])
		if m["toolUseId"] != r.Subagents.Calls[call] {
			t.Fatalf("%s = %v, want toolUseId %s", key, m, r.Subagents.Calls[call])
		}
		if _, has := m["finished"]; has != finished {
			t.Fatalf("%s = %v, want finished present = %v", key, m, finished)
		}
		return m
	}
	fileOf := func(t *testing.T, r *running, key string) string {
		t.Helper()
		raw, err := os.ReadFile(r.TaskFiles[key])
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	const handback = `"name":"SubagentHandback"`
	stopped := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubStoppedMetaKey, "stopped", false)
		if strings.Contains(fileOf(t, r, bgtx.SubStoppedFileKey), handback) {
			t.Fatal("stopped file has a handback")
		}
	}
	background := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		if m := metaOf(t, r, bgtx.SubBackgroundMetaKey, "background", false); m["requestShape"] != "background" {
			t.Fatalf("background meta = %v, want requestShape background", m)
		}
		// async_launched links the file, so the row has what the file holds.
		for _, row := range chat.Tasks {
			if row.Label == bgtx.SubBackgroundDesc {
				if row.Finished || row.Model == "" || row.Tokens == 0 || row.ToolCalls == 0 {
					t.Fatalf("background row = %+v, want running with model, tokens and tool calls", row)
				}
				return
			}
		}
		t.Fatalf("no task row %q in %+v", bgtx.SubBackgroundDesc, chat.Tasks)
	}
	nested := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubNestedMetaKey, "nested", false)
		child := metaOf(t, r, bgtx.SubGrandchildMetaKey, "grandchild", false)
		if child["parentAgentId"] != bgtx.SubNestedAgent {
			t.Fatalf("grandchild meta = %v, want parentAgentId %s", child, bgtx.SubNestedAgent)
		}
		if !strings.Contains(fileOf(t, r, bgtx.SubNestedFileKey), `"id":"`+r.Subagents.Calls["grandchild"]+`"`) {
			t.Fatalf("nested file lacks the call %s", r.Subagents.Calls["grandchild"])
		}
		// Flat in one folder, as Claude Code writes them.
		if filepath.Dir(r.TaskFiles[bgtx.SubNestedFileKey]) != filepath.Dir(r.TaskFiles[bgtx.SubGrandchildFileKey]) {
			t.Fatalf("nested files %v are not in one folder", r.TaskFiles)
		}
	}
	starting := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		for _, key := range []string{bgtx.SubStartingFileKey, bgtx.SubStartingMetaKey} {
			path := r.TaskFiles[key]
			if path == "" {
				t.Fatalf("task files %v lack %s", r.TaskFiles, key)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s exists at start: %v", path, err)
			}
			if _, err := os.Stat(filepath.Dir(path)); err != nil {
				t.Fatalf("folder of %s: %v", key, err)
			}
		}
	}
	marked := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubMarkedMetaKey, "marked", true)
		file := fileOf(t, r, bgtx.SubMarkedFileKey)
		if strings.Contains(file, handback) || !strings.Contains(file, bgtx.SubMarkedLast) {
			t.Fatalf("marked file = %s, want the last message and no handback", file)
		}
	}
	// The running subagent's meta, for the shapes whose session is gone or exited.
	runningMeta := func(t *testing.T, r *running, _ wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubRunningMetaKey, "running", false)
	}
	cases := []shape{
		{"claude", []string{"running", "finished", "missing"}, func(t *testing.T, r *running, c wire.SessionChatResponse) {
			checkRunning(t, r, c)
			finished(t, r, c)
		}},
		{"foreground-running", []string{"running"}, checkRunning},
		{"finished-handback", []string{"finished"}, finished},
		{"file-gone", []string{"missing"}, none},
		{"stopped-no-handback", []string{"stopped"}, stopped},
		{"background-returned", []string{"background"}, background},
		{"nested", []string{"nested"}, nested},
		{"starting", []string{"starting"}, starting},
		{"finished-no-handback", []string{"marked"}, marked},
		// The parent's transcript is deleted: the chat read has no tools entry, but reads.
		{"parent-gone", nil, runningMeta},
		{"exited", []string{"running"}, runningMeta},
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
