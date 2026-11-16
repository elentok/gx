package spec

import (
	"fmt"
	"time"

	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// BgProgressShapes are the --shape names of the bg-progress scenario; the first
// is the default. The ones in bgBuilders are built; the rest are not yet.
var BgProgressShapes = []string{
	"claude", "codex", "old-codex", "single-task", "exited", "no-end",
	"subagent-with-inner-task", "many", "timed-out-shell", "growing-output",
}

// Names the tests and flows of the shapes look at.
const (
	// BgTimedOutUseID is the tool_use id of the timed-out-shell call and of the
	// claude shape's moved-to-background call.
	BgTimedOutUseID = "toolu_timedout"
	// BgInnerAgentID is the subagent of the subagent-with-inner-task shape.
	BgInnerAgentID = "a1ner0001"
	// BgGrowingKey is the growing-output shell's key in Output.TaskFiles.
	BgGrowingKey = "growing-output"
	// BgMonitorEvents is the number of event notices of the claude shape's Monitor.
	BgMonitorEvents = 6
	// BgManyTasks and BgManyFinished are the many shape's rows.
	BgManyTasks, BgManyFinished = 50, 20
	// BgScriptLines is the line count of the long shell commands.
	BgScriptLines = 16
)

// bgBuilders maps a shape to its expected task count and its lines.
var bgBuilders = map[string]struct {
	tasks int
	build func(b *bgLines)
}{
	"claude":                   {7, bgClaude},
	"single-task":              {1, bgSingleTask},
	"subagent-with-inner-task": {1, bgInnerTask},
	"many":                     {BgManyTasks, bgMany},
	"timed-out-shell":          {1, bgTimedOut},
	"growing-output":           {1, bgGrowing},
}

// BgProgressScenario is the bg-progress scenario in the given shape: one
// Working Claude Design session of design-epic whose transcript and task files
// hold the shape's tasks. False when the shape is not built yet.
func BgProgressScenario(shape string) (Scenario, bool) {
	b, ok := bgBuilders[shape]
	if !ok {
		return Scenario{}, false
	}
	const mapAddr = project + ":design-epic"
	return Scenario{
		Epics:       []Epic{{Project: project, Name: "design-epic"}},
		DesignSteps: map[string]string{mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Sessions: []SessionSpec{{
			Project: project, Prompt: agentfake.ChatSlowMarker + " background tasks", Link: mapAddr, Key: shape,
			BgTasks: b.tasks,
			BgSeed: func(now time.Time, p BgPaths) BgSeed {
				l := newBgLines(now, p)
				b.build(l)
				return l.seed()
			},
		}},
	}, true
}

const (
	sec    = time.Second
	minute = time.Minute
)

// bgClaude: 7 tasks in one group, 3 done. The subagent runs throughout.
func bgClaude(b *bgLines) {
	b.subagent(6*minute, BgAgentID, "Survey the sessions code", surveyFile())

	b.shell(5*minute+50*sec, "toolu_lint", "blint0001", "make lint", outputText("lint", 4, 40), 5*minute+8*sec)
	b.endShell(5*minute+8*sec, "blint0001", "completed", `Background command "make lint" completed (exit code 0)`)

	b.shell(5*minute, "toolu_script", "bscript01", bashScript(BgScriptLines), outputText("script", 5, 40)+"[exited with code 2]\n", 4*minute+48*sec)
	b.endShell(4*minute+48*sec, "bscript01", "failed", `Background command "bash script" failed with exit code 2`)

	b.shell(4*minute+30*sec, "toolu_sleep", "bsleep001", "sleep 900", "", 4*minute+18*sec)
	b.use(4*minute+19*sec, "toolu_stop", "TaskStop", map[string]any{"task_id": "bsleep001"})
	b.result(4*minute+18*sec, "toolu_stop", "Stopped bsleep001", map[string]any{"task_id": "bsleep001"})

	b.use(4*minute, BgTimedOutUseID, "Bash", map[string]any{"command": "python3 - <<'PY'\nimport time\nfor i in range(100):\n    print(i)\n    time.sleep(5)\nPY", "timeout": 120000})
	b.result(2*minute, BgTimedOutUseID, "Command exceeded the timeout and moved to the background. Output is being written to: "+b.outputPath("bpython01"),
		map[string]any{"backgroundTaskId": "bpython01", "timedOutAfterMs": 120000})
	b.file(BgFile{Path: b.outputPath("bpython01"), Content: outputText("python", 8, 20), Mtime: b.when(30 * sec)})

	b.use(3*minute, "toolu_monitor", "Monitor", map[string]any{"command": "tail -f logs/app.log", "timeout_ms": 600000, "description": "Watch the app log"})
	b.result(3*minute-sec, "toolu_monitor", "Monitor started: bmon00001", map[string]any{"taskId": "bmon00001", "timeoutMs": 600000})
	for i := range BgMonitorEvents {
		b.notify(170*sec-time.Duration(i)*20*sec, fmt.Sprintf("<task-id>bmon00001</task-id>\n<summary>Monitor event: line %d</summary>\n<event>request %d served</event>", i+1, i+1))
	}

	b.use(minute, "toolu_test", "Bash", map[string]any{"command": "make test"})
}

func bgSingleTask(b *bgLines) {
	b.shell(minute, "toolu_long", "blong0001", bashScript(BgScriptLines), outputText("long", 3, 30), 20*sec)
}

// bgInnerTask: the subagent's own file launches a shell the parent never names.
func bgInnerTask(b *bgLines) {
	inner := []string{
		subAssistant("msg_i1", 900, 50, subUse("toolu_inner", "Bash", map[string]any{"command": "make watch", "run_in_background": true})),
		subResult("toolu_inner", "Command running in background with ID: binner001", map[string]any{"backgroundTaskId": "binner001"}),
	}
	b.subagent(4*minute, BgInnerAgentID, "Watch the build", inner)
}

// bgMany: the anchor subagent, 29 running shells and 20 finished ones.
func bgMany(b *bgLines) {
	b.subagent(6*minute, BgAgentID, "Survey the sessions code", surveyFile())
	for i := range BgManyTasks - 1 {
		id := fmt.Sprintf("bmany%04d", i)
		start := 5*minute - time.Duration(i)*5*sec
		b.shell(start, "toolu_"+id, id, fmt.Sprintf("make step-%d", i), outputText(id, 40, 50), 20*sec)
		if i < BgManyFinished {
			b.endShell(start-3*sec, id, "completed", fmt.Sprintf(`Background command "make step-%d" completed (exit code 0)`, i))
		}
	}
}

// bgTimedOut: the call ran past its timeout and moved to the background.
func bgTimedOut(b *bgLines) {
	b.use(9*minute, BgTimedOutUseID, "Bash", map[string]any{"command": "make slow-test", "timeout": 120000})
	b.result(2*minute, BgTimedOutUseID, "Command exceeded the timeout and moved to the background. Output is being written to: "+b.outputPath("btimeout1"),
		map[string]any{"backgroundTaskId": "btimeout1", "timedOutAfterMs": 120000})
	b.file(BgFile{Path: b.outputPath("btimeout1"), Content: outputText("slow", 3, 30), Mtime: b.when(30 * sec)})
}

// bgGrowing: one running shell whose output file a test grows.
func bgGrowing(b *bgLines) {
	b.shell(minute, "toolu_grow", "bgrow0001", "make watch", "one\ntwo\nthree\n", 20*sec)
	b.files[len(b.files)-1].Key = BgGrowingKey
}
