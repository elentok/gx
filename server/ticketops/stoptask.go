package ticketops

import (
	"errors"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/transcript"
)

// stopAgentTaskPrompt asks Claude to end an async Agent task itself.
func stopAgentTaskPrompt(taskID string) string {
	return "Use the TaskStop tool with task_id " + taskID + " to stop that background agent task. Then end your turn with the single word stopped."
}

// StopTask ends the live background task of a background-task-gate pause. A
// shell task's process is signalled and nothing is typed: Claude writes its own
// failed notice and wakes. An async Agent task is stopped by asking Claude to
// run TaskStop. Exit codes are never read. The wait then continues as for
// Keep waiting.
func (o *Ops) StopTask(req wire.QueueRequest) (wire.QueueResult, error) {
	return o.gateVerb(req, "task stopped by a person", func(run runs.TrackedRun, _ string) (*wire.QueueResult, error) {
		if run.Session.SessionID == "" || agent.AgentFor(run.Agent) != agent.AgentClaude {
			r := queue.Refusal(wire.ReasonNotStoppable, "only a Claude task can be stopped")
			return &r, nil
		}
		home, err := o.d.Projects.EnvHome()
		if err != nil {
			return nil, err
		}
		rd := &backgroundtask.Reader{Home: home}
		f, ok := rd.Facts(run.Session.SessionID, backgroundtask.DefaultOutputBase(), backgroundtask.ProcessFinder{Procs: syshealth.DefaultProcess()})
		if !ok {
			r := queue.Refusal(wire.ReasonTaskUnidentified, "no live task found to stop")
			return &r, nil
		}
		if f.Kind == transcript.BackgroundTaskAgent {
			runner, err := o.d.Harness.RunnerOf(run.Run)
			if err != nil {
				return nil, err
			}
			switch err := runner.Prompt(run.Session, stopAgentTaskPrompt(f.TaskID)); {
			case errors.Is(err, agentrunner.ErrNotReady):
				r := queue.Refusal(wire.ReasonAgentNotReady, err.Error())
				return &r, nil
			case err != nil:
				return nil, err
			}
			return nil, nil
		}
		if !f.Process.Alive {
			msg := "cannot tell which process belongs to the task"
			if f.Process.Ambiguous {
				msg = "several processes fit the task; not guessing"
			}
			r := queue.Refusal(wire.ReasonTaskUnidentified, msg)
			return &r, nil
		}
		return nil, backgroundtask.Stop(f.Process.PID, backgroundtask.StopGrace)
	})
}
