package herdrfake

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
)

// trackedAgent is what trackAgents remembers of one pane's agent from the
// responses handler gave.
type trackedAgent struct {
	name, tab, status string
	session           any
	seq               int
}

// trackAgents wraps h so a hand-rolled handler answers the way real herdr
// does for an agentrunner adapter: agent responses missing state_change_seq
// or tab_id get them filled in, and "agent get" is answered from the last
// response seen (or a timed-out wait) when h doesn't implement it. The seq advances on every
// status change and every prompt, like herdr's.
func trackAgents(h Handler) Handler {
	var mu sync.Mutex
	tabs := map[string]string{}
	agents := map[string]*trackedAgent{}
	find := func(target string) (string, *trackedAgent) {
		if a := agents[target]; a != nil {
			return target, a
		}
		for pane, a := range agents {
			if a.name == target {
				return pane, a
			}
		}
		return "", nil
	}
	return func(argv []string) ([]byte, int) {
		out, code := h(argv)
		if len(argv) < 3 {
			return out, code
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case argv[0] == "tab" && argv[1] == "create" && code == 0:
			var r struct {
				Result struct {
					Tab struct {
						TabID string `json:"tab_id"`
					} `json:"tab"`
					RootPane struct {
						PaneID string `json:"pane_id"`
					} `json:"root_pane"`
				} `json:"result"`
			}
			if json.Unmarshal(out, &r) == nil {
				tabs[r.Result.RootPane.PaneID] = r.Result.Tab.TabID
			}
			return out, code
		case argv[0] != "agent":
			return out, code
		case argv[1] == "get" && code != 0 && strings.Contains(string(out), "unimplemented command"):
			pane, a := find(argv[2])
			if a == nil {
				return CommandError(errorEnvelope("agent_not_found", "agent target "+argv[2]+" not found").Error())
			}
			return Result(map[string]any{"agent": a.info(pane)})
		case argv[1] == "wait" && code != 0 && strings.Contains(string(out), "timed out"):
			// A wait for anything but "working" timing out is the first sign
			// of an agent already mid-turn.
			if _, a := find(argv[2]); a == nil && !slices.Contains(flags(argv, "--until"), "working") {
				agents[argv[2]] = &trackedAgent{tab: tabs[argv[2]], status: "working", seq: 1}
			}
			return out, code
		case code != 0:
			return out, code
		}
		var r struct {
			Result struct {
				Agent map[string]any `json:"agent"`
			} `json:"result"`
		}
		if json.Unmarshal(out, &r) != nil || r.Result.Agent == nil {
			return out, code
		}
		agent := r.Result.Agent
		pane, _ := agent["pane_id"].(string)
		if pane == "" {
			pane, _ = find(argv[2])
		}
		if pane == "" {
			return out, code
		}
		a := agents[pane]
		if a == nil {
			a = &trackedAgent{tab: tabs[pane]}
			agents[pane] = a
		}
		if argv[1] == "start" {
			a.name = argv[2]
		}
		status, _ := agent["agent_status"].(string)
		if status != a.status || argv[1] == "prompt" {
			a.seq++
		}
		a.status = status
		if s, ok := agent["agent_session"]; ok {
			a.session = s
		}
		if seq, ok := agent["state_change_seq"].(float64); ok {
			a.seq = int(seq)
		}
		for k, v := range a.info(pane) {
			if _, ok := agent[k]; !ok {
				agent[k] = v
			}
		}
		return Result(map[string]any{"agent": agent})
	}
}

func (a *trackedAgent) info(pane string) map[string]any {
	info := map[string]any{"name": a.name, "pane_id": pane, "agent_status": a.status, "state_change_seq": a.seq}
	if a.tab != "" {
		info["tab_id"] = a.tab
	}
	if a.session != nil {
		info["agent_session"] = a.session
	}
	return info
}
