package bgtx

import (
	"fmt"
	"time"
)

// Names the tests and flows of the shapes look at.
const (
	TimedOutUseID = "toolu_timedout" // tool_use id of the timed-out-shell and claude shapes' moved-to-background call
	InnerAgentID  = "a1ner0001"      // the subagent of subagent-with-inner-task
	GrowingKey    = "growing-output" // the growing-output shell's key in Output.TaskFiles
	MonitorEvents = 6                // event notices of the claude shape's Monitor
	ScriptLines   = 16               // line count of the long shell commands

	ManyTasks, ManyFinished = 50, 20 // the many shape's rows
)

// Single maps a single-session shape to its task count and lines.
var Single = map[string]struct {
	Tasks int
	Build func(b *Lines)
}{
	"claude":                   {7, claude},
	"single-task":              {1, singleTask},
	"subagent-with-inner-task": {1, innerTask},
	"many":                     {ManyTasks, many},
	"timed-out-shell":          {1, timedOutShape},
	"growing-output":           {1, growing},
}

// claude: 7 tasks in one group, 3 done. The subagent runs throughout.
func claude(b *Lines) {
	b.subagent(6*minute, agentID, "Survey the sessions code", surveyFile())
	b.shell(5*minute+50*sec, "toolu_lint", "blint0001", "make lint", outputText("lint", 4, 40), 5*minute+8*sec)
	b.endShell(5*minute+8*sec, "blint0001", "completed", `Background command "make lint" completed (exit code 0)`)
	b.shell(5*minute, "toolu_script", "bscript01", bashScript(ScriptLines), outputText("script", 5, 40)+"[exited with code 2]\n", 4*minute+48*sec)
	b.endShell(4*minute+48*sec, "bscript01", "failed", `Background command "bash script" failed with exit code 2`)
	b.shell(4*minute+30*sec, "toolu_sleep", "bsleep001", "sleep 900", "", 4*minute+18*sec)
	b.use(4*minute+19*sec, "toolu_stop", "TaskStop", map[string]any{"task_id": "bsleep001"})
	b.result(4*minute+18*sec, "toolu_stop", "Stopped bsleep001", map[string]any{"task_id": "bsleep001"})
	b.timedOut(4*minute, 2*minute, TimedOutUseID, "bpython01", "python3 - <<'PY'\nimport time\nfor i in range(100):\n    print(i)\n    time.sleep(5)\nPY", outputText("python", 8, 20))
	b.monitor(3*minute, "toolu_monitor", "bmon00001", 600000)
	for i := range MonitorEvents {
		b.notify(170*sec-time.Duration(i)*20*sec, fmt.Sprintf("<task-id>bmon00001</task-id>\n<summary>Monitor event: line %d</summary>\n<event>request %d served</event>", i+1, i+1))
	}
	b.use(minute, "toolu_test", "Bash", map[string]any{"command": "make test"})
}

func singleTask(b *Lines) {
	b.shell(minute, "toolu_long", "blong0001", bashScript(ScriptLines), outputText("long", 3, 30), 20*sec)
}

// innerTask: the subagent's own file launches a shell the parent never names.
func innerTask(b *Lines) {
	b.subagent(4*minute, InnerAgentID, "Watch the build", []string{
		subAssistant("msg_i1", 900, 50, toolUse("toolu_inner", "Bash", map[string]any{"command": "make watch", "run_in_background": true})),
		subResult("toolu_inner", "Command running in background with ID: binner001", map[string]any{"backgroundTaskId": "binner001"}),
	})
}

// many: the anchor subagent, 29 running shells and 20 finished ones.
func many(b *Lines) {
	b.subagent(6*minute, agentID, "Survey the sessions code", surveyFile())
	for i := range ManyTasks - 1 {
		id := fmt.Sprintf("bmany%04d", i)
		start := 5*minute - time.Duration(i)*5*sec
		b.shell(start, "toolu_"+id, id, fmt.Sprintf("make step-%d", i), outputText(id, 40, 50), 20*sec)
		if i < ManyFinished {
			b.endShell(start-3*sec, id, "completed", fmt.Sprintf(`Background command "make step-%d" completed (exit code 0)`, i))
		}
	}
}

// timedOutShape: the call ran past its timeout and moved to the background.
func timedOutShape(b *Lines) {
	b.timedOut(9*minute, 2*minute, TimedOutUseID, "btimeout1", "make slow-test", outputText("slow", 3, 30))
}

// growing: one running shell whose output file a test grows.
func growing(b *Lines) {
	b.shell(minute, "toolu_grow", "bgrow0001", "make watch", "one\ntwo\nthree\n", 20*sec)
	b.files[len(b.files)-1].Key = GrowingKey
}
