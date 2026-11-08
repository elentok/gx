package ticketops

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// stopGateAgent stops the agent of a background-task-gate pause that names a
// live task, and its task's process, and confirms them gone. Attaching to an
// agent that waits on a hung task would do nothing, so Retry replaces it. It
// reports whether it stopped one. A gate pause naming no task (the
// dirty-worktree variant), or any other pause, is left alone.
func (o *Ops) stopGateAgent(address string) (bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil || o.d.Registry == nil {
		return false, nil
	}
	dir, err := o.d.Projects.Dir(addr.Project)
	if err != nil {
		return false, nil
	}
	_, t, ok := o.d.Writer.FreshTicket(addr)
	if !ok {
		return false, nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() || tk.PauseReason != schema.PauseBackgroundTaskGate {
		return false, nil
	}
	if taskID, err := pausedTaskID(queue.TicketRef{Addr: addr, ProjectDir: dir}); err != nil || taskID == "" {
		return false, err
	}
	if run, ok := o.d.Registry.Tracked(addr.String()); ok && run.Session.SessionID != "" && agent.AgentFor(run.Agent) == agent.AgentClaude {
		// Best effort: the agent is stopped next either way.
		if f, ok, err := o.taskFacts(run); err == nil && ok && f.Kind != transcript.BackgroundTaskAgent && f.Process.Alive {
			_ = backgroundtask.Stop(f.Process.PID, backgroundtask.StopGrace)
		}
	}
	anyLive := func(agentrunner.Runner, agentrunner.Session) bool { return true }
	_, _, err = o.SettleLive([]tickets.Address{addr}, true, wire.ReasonTicketLive, anyLive)
	return err == nil, err
}

// Retry serves the retry route. Retry on an answer-class pause whose agent is
// still running prompts that agent through the resume dispatch (status
// prompted); a live-task gate pause stops its agent and task tree and reopens
// the ticket through the same dispatch; every other retry relaunches. An
// investigator's retry always relaunches.
func (o *Ops) Retry(web bool) http.HandlerFunc {
	relaunch := queue.HandlerAs(web, o.d.Writer.Relaunch)
	resume := o.ResumeHandler(web)
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var req struct {
			Address string `json:"address"`
		}
		parsed := json.Unmarshal(body, &req) == nil
		person := parsed && (web || r.Header.Get(wire.InvestigateHeader) == "")
		if person {
			stopped, err := o.stopGateAgent(req.Address)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if stopped {
				resume(w, r)
				return
			}
		}
		if person && o.PromptsOnRetry(req.Address) {
			resume(w, r)
			return
		}
		relaunch(w, r)
	}
}
