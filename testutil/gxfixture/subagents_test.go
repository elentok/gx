package main

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
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
		"stopped": bgtx.SubStoppedDesc, "background": bgtx.SubBackgroundDesc, "nested": bgtx.SubNestedDesc, "starting": bgtx.SubStartingDesc, "marked": bgtx.SubMarkedDesc,
		"string": bgtx.SubStringDesc, "read": bgtx.SubReadDesc, "edit": bgtx.SubEditDesc,
		"tickets": bgtx.CardTicketsDesc, "names": bgtx.CardNamesDesc, "probe": bgtx.CardProbeDesc}
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
	stringBackground := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubStringMetaKey, "string", false)
		rows := map[string]wire.SessionTask{}
		for _, row := range chat.Tasks {
			rows[row.Label] = row
		}
		if len(chat.Tasks) != 2 || rows[bgtx.SubStringDesc].CallID != r.Subagents.Calls["string"] || rows[bgtx.SubStringShellDesc].Kind != "shell" {
			t.Fatalf("task rows = %+v, want the subagent row with its call id and the shell row", chat.Tasks)
		}
	}
	fileCall := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		metaOf(t, r, bgtx.SubReadMetaKey, "read", false)
		metaOf(t, r, bgtx.SubEditMetaKey, "edit", false)
		got := map[string]string{}
		for _, row := range chat.Tasks {
			got[row.Label] = row.CurrentCall
			if !strings.HasSuffix(row.CurrentCall, " "+row.CurrentCallPath) {
				t.Errorf("row %q path = %q, want the end of %q", row.Label, row.CurrentCallPath, row.CurrentCall)
			}
		}
		want := map[string]string{bgtx.SubReadDesc: bgtx.SubReadLabel, bgtx.SubEditDesc: bgtx.SubEditLabel}
		if !maps.Equal(got, want) {
			t.Errorf("current calls = %q, want %q", got, want)
		}
	}
	// openCalls are the tool_use ids of a subagent file that have no result.
	openCalls := func(t *testing.T, r *running, key string) []string {
		t.Helper()
		var uses []string
		done := map[string]bool{}
		for line := range strings.Lines(fileOf(t, r, key)) {
			var l struct {
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(line), &l); err != nil {
				t.Fatal(err)
			}
			var blocks []struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				ToolUseID string `json:"tool_use_id"`
			}
			// A prompt line's content is a string.
			if json.Unmarshal(l.Message.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				switch b.Type {
				case "tool_use":
					uses = append(uses, b.ID)
				case "tool_result":
					done[b.ToolUseID] = true
				}
			}
		}
		return slices.DeleteFunc(uses, func(id string) bool { return done[id] })
	}
	// cardRows checks the chat read's rows are exactly the wanted subagents, in launch order.
	cardRows := func(t *testing.T, chat wire.SessionChatResponse, labels ...string) map[string]wire.SessionTask {
		t.Helper()
		sorted := slices.SortedFunc(slices.Values(chat.Tasks), func(a, b wire.SessionTask) int { return strings.Compare(a.StartedAt, b.StartedAt) })
		var got []string
		rows := map[string]wire.SessionTask{}
		for _, row := range sorted {
			if row.Kind != "subagent" {
				t.Errorf("row %q kind = %q, want subagent", row.Label, row.Kind)
			}
			got = append(got, row.Label)
			rows[row.Label] = row
		}
		if !slices.Equal(got, labels) {
			t.Fatalf("task rows by start = %q, want %q", got, labels)
		}
		return rows
	}
	runningCard := func(t *testing.T, r *running, row wire.SessionTask, call, fileKey, openID string) {
		t.Helper()
		if row.Finished || row.CurrentCall != call {
			t.Errorf("row = %+v, want running %q", row, call)
		}
		if got := openCalls(t, r, fileKey); !slices.Equal(got, []string{openID}) {
			t.Errorf("%s open calls = %q, want only %q", fileKey, got, openID)
		}
	}
	cardTickets := func(t *testing.T, r *running, row wire.SessionTask) {
		t.Helper()
		metaOf(t, r, bgtx.CardTicketsMetaKey, "tickets", false)
		runningCard(t, r, row, "Read tickets/frontier.go", bgtx.CardTicketsFileKey, bgtx.CardTicketsOpenCall)
	}
	cardTwo := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		rows := cardRows(t, chat, bgtx.CardTicketsDesc, bgtx.CardNamesDesc, bgtx.CardProbeDesc)
		cardTickets(t, r, rows[bgtx.CardTicketsDesc])
		metaOf(t, r, bgtx.CardProbeMetaKey, "probe", false)
		runningCard(t, r, rows[bgtx.CardProbeDesc], "rg -n spawn_agent codexsession", bgtx.CardProbeFileKey, bgtx.CardProbeOpenCall)
		metaOf(t, r, bgtx.CardNamesMetaKey, "names", true)
		done := rows[bgtx.CardNamesDesc]
		if !done.Finished || done.Outcome != "success" {
			t.Errorf("row = %+v, want finished with success", done)
		}
		start, err := time.Parse(time.RFC3339Nano, done.StartedAt)
		if err != nil {
			t.Fatal(err)
		}
		end, err := time.Parse(time.RFC3339Nano, done.FinishedAt)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := end.Sub(start), 4*time.Minute+51*time.Second; got != want {
			t.Errorf("duration = %s, want %s", got, want)
		}
	}
	cardOne := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		cardTickets(t, r, cardRows(t, chat, bgtx.CardTicketsDesc)[bgtx.CardTicketsDesc])
	}
	cardNone := func(t *testing.T, r *running, chat wire.SessionChatResponse) {
		if len(chat.Tasks) != 0 {
			t.Errorf("task rows = %+v, want none", chat.Tasks)
		}
		if r.Subagents.SessionID == "" || r.Subagents.ParentPath == "" {
			t.Errorf("output subagents = %+v, want the session id and parent path", r.Subagents)
		}
		list := r.sessions(t)
		if len(list) != 1 || list[0].Status.State != wire.SessionWorking {
			t.Errorf("sessions = %+v, want one Working session", list)
		}
	}
	cases := []shape{
		{"card-two-running", []string{"tickets", "names", "probe"}, cardTwo},
		{"card-one-subagent", []string{"tickets"}, cardOne},
		{"card-no-tasks", nil, cardNone},
		{"file-call", []string{"read", "edit"}, fileCall},
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
		{"string-background", []string{"string"}, stringBackground},
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

func TestSubagents_CodexShapes(t *testing.T) {
	t.Parallel()
	// rolloutLines reads the child's rollout lines; the path is in the output either way.
	rolloutLines := func(t *testing.T, r *running) []string {
		raw, err := os.ReadFile(r.TaskFiles[bgtx.CodexSubRolloutKey])
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(raw)), "\n")
	}
	// checkChild checks the session_meta, the history before the ordinal and the own lines after it.
	checkChild := func(t *testing.T, r *running, wantRole any) []string {
		lines := rolloutLines(t, r)
		var meta struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
			t.Fatal(err)
		}
		p := meta.Payload
		role, hasRole := p["agent_role"]
		if !hasRole || role != wantRole || p["thread_source"] != "subagent" || p["id"] != r.Subagents.Calls["thread"] {
			t.Fatalf("session_meta = %v, want agent_role %v, thread_source subagent and the thread id", p, wantRole)
		}
		ordinal, ok := p["subagent_history_start_ordinal"].(float64)
		if !ok || int(ordinal) < 3 || int(ordinal) >= len(lines) {
			t.Fatalf("session_meta = %v, want a subagent_history_start_ordinal inside the file", p)
		}
		if !strings.Contains(lines[int(ordinal)-1], bgtx.CodexSubMarker) || !strings.Contains(lines[1], "Please review the change.") {
			t.Fatalf("lines before the ordinal = %v, want the parent history ending in the marker", lines[:int(ordinal)])
		}
		own := lines[int(ordinal):]
		if !strings.Contains(own[0], `"agent_message"`) || !strings.Contains(strings.Join(own, "\n"), bgtx.CodexSubLast) {
			t.Fatalf("own lines = %v, want the prompt first and the last message", own)
		}
		return own
	}
	home := func(r *running) string { return filepath.Dir(filepath.Dir(r.TaskFiles[bgtx.CodexSubRolloutKey])) }
	cases := []struct {
		name  string
		check func(t *testing.T, r *running, row wire.SessionTask)
	}{
		{"codex-starting", func(t *testing.T, r *running, row wire.SessionTask) {
			if _, err := os.Stat(r.TaskFiles[bgtx.CodexSubRolloutKey]); !os.IsNotExist(err) {
				t.Fatalf("child rollout exists at start: %v", err)
			}
			if _, ok, err := codexsession.RolloutPathIn(home(r), r.Subagents.Calls["thread"]); ok || err != nil {
				t.Fatalf("RolloutPathIn = %v, %v, want none", ok, err)
			}
			if row.Finished {
				t.Fatalf("row = %+v, want it counted as running", row)
			}
		}},
		{"codex-child", func(t *testing.T, r *running, row wire.SessionTask) {
			checkChild(t, r, "reviewer")
			if row.Finished || row.Model != bgtx.CodexChildModel || row.Tokens != bgtx.CodexChildTokens {
				t.Fatalf("row = %+v, want running with the child's model and tokens", row)
			}
		}},
		{"codex-null-role", func(t *testing.T, r *running, _ wire.SessionTask) { checkChild(t, r, nil) }},
		{"codex-compressed", func(t *testing.T, r *running, _ wire.SessionTask) {
			thread := r.Subagents.Calls["thread"]
			zst, ok, err := codexsession.ArchivedRolloutIn(home(r), thread)
			if err != nil || !ok || !strings.HasSuffix(zst, ".jsonl.zst") || !strings.Contains(filepath.Base(zst), thread) {
				t.Fatalf("ArchivedRolloutIn = %q, %v, %v, want the .jsonl.zst", zst, ok, err)
			}
			if path, ok, err := codexsession.RolloutPathIn(home(r), thread); ok || err != nil {
				t.Fatalf("RolloutPathIn = %q, %v, %v, want none", path, ok, err)
			}
		}},
		{"codex-running-call", func(t *testing.T, r *running, row wire.SessionTask) {
			if history := rolloutLines(t, r)[:4]; !strings.Contains(strings.Join(history, "\n"), bgtx.CodexSubInheritedCmd) {
				t.Fatalf("history = %v, want the inherited open call", history)
			}
			if row.Finished || row.CurrentCall != bgtx.CodexSubRunningCmd {
				t.Fatalf("row = %+v, want running %q", row, bgtx.CodexSubRunningCmd)
			}
		}},
		{"codex-nested", func(t *testing.T, r *running, _ wire.SessionTask) {
			if own := strings.Join(checkChild(t, r, "reviewer"), "\n"); !strings.Contains(own, `"name":"spawn_agent"`) || !strings.Contains(own, bgtx.CodexSubGrandcall) {
				t.Fatalf("own lines = %s, want a spawn_agent call", own)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "subagents", nil, "--shape", tc.name)
			chat := r.subChat(t)
			if r.Subagents.Calls["spawn"] == "" || r.Subagents.Calls["thread"] == "" || r.Subagents.ParentPath == "" {
				t.Fatalf("output subagents = %+v, want spawn and thread ids and the parent path", r.Subagents)
			}
			var row wire.SessionTask
			for _, task := range chat.Tasks {
				if task.Label == bgtx.CodexSubName {
					row = task
				}
			}
			if row.Label == "" {
				t.Fatalf("no task row %q in %+v", bgtx.CodexSubName, chat.Tasks)
			}
			tc.check(t, r, row)
		})
	}
}

// An old rollout's spawn_agent call is marked, but has no record and no task row.
func TestSubagents_OldCodexCall(t *testing.T) {
	t.Parallel()
	r := startWith(t, "subagents", nil, "--shape", "codex-old-call")
	chat := r.subChat(t)
	call := r.Subagents.Calls["spawn"]
	if call == "" {
		t.Fatalf("output calls = %v, want spawn", r.Subagents.Calls)
	}
	var marks []wire.SessionChatSubagentCall
	for _, m := range chat.Messages {
		if m.Kind == "tools" {
			marks = append(marks, m.SubagentCalls...)
		}
	}
	if len(marks) != 1 || marks[0].CallID != call || marks[0].State != wire.SubagentCallNoRecord {
		t.Errorf("subagent marks = %+v, want %s with no record", marks, call)
	}
	if len(chat.Tasks) != 0 {
		t.Errorf("task rows = %+v, want none", chat.Tasks)
	}
	raw, err := os.ReadFile(r.Subagents.ParentPath)
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(string(raw), "\n"); !strings.Contains(first, `"cli_version":"`+bgtx.CodexOldVersion+`"`) {
		t.Errorf("parent session_meta = %s, want cli_version %s", first, bgtx.CodexOldVersion)
	}
	list := r.sessions(t)
	if len(list) != 1 || list[0].Harness != "codex" || list[0].FlowKey != "demo:design-epic" {
		t.Errorf("sessions = %+v, want one Codex session linked to demo:design-epic", list)
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
