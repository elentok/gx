package herdrrunner

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/herdr"
)

// The lookups below take herdr's calls as funcs so ralphloop can drive them
// through its own Deps fakes until it moves onto the Runner.

// FindTab returns the tab labeled label among workspaceID's tabs.
func FindTab(tabList func(workspaceID string) ([]herdr.Tab, error), workspaceID, label string) (herdr.Tab, bool, error) {
	tabs, err := tabList(workspaceID)
	if err != nil {
		return herdr.Tab{}, false, err
	}
	for _, tab := range tabs {
		if tab.Label == label {
			return tab, true, nil
		}
	}
	return herdr.Tab{}, false, nil
}

// ErrAgentElsewhere: the named agent is live, but not in the expected tab.
var ErrAgentElsewhere = errors.New("agent runs outside its tab")

// OwnedAgent reads the agent named name and checks it runs in workspaceID's
// tab tabID: a name alone could belong to an unrelated launch.
func OwnedAgent(agentGet func(target string) (herdr.Agent, error), name, workspaceID, tabID string) (herdr.Agent, error) {
	agent, err := agentGet(name)
	if err != nil {
		return herdr.Agent{}, fmt.Errorf("reading agent %s: %w", name, err)
	}
	if agent.PaneID == "" || agent.TabID != tabID || agent.WorkspaceID != workspaceID {
		return herdr.Agent{}, fmt.Errorf("%w: %s, want workspace %s tab %s", ErrAgentElsewhere, name, workspaceID, tabID)
	}
	return agent, nil
}

// StartRecovery says how a launch goes on after AgentStart failed.
type StartRecovery int

const (
	// StartAdopt: our own earlier launch in the same cwd holds the name.
	StartAdopt StartRecovery = iota + 1
	// StartResume: gx's trust_directory dialog was answered, and the agent is
	// coming up in its pane.
	StartResume
)

// RecoverStart decides how to go on after AgentStart(name) in pane, launched
// in cwd, failed with err. Errors it can't recover from come back wrapped in
// agentrunner.ErrLabelTaken or ErrNotReady where one applies.
func RecoverStart(
	err error,
	name, cwd, pane string,
	explain func(target string) (herdr.AgentExplainResult, error),
	sendKeys func(target string, keys ...string) error,
) (StartRecovery, error) {
	var taken *herdr.AgentNameTakenError
	var notReady *herdr.AgentNotReadyError
	switch {
	case errors.As(err, &taken):
		if taken.CandidateCwd != "" && taken.CandidateCwd == cwd {
			return StartAdopt, nil
		}
		return 0, fmt.Errorf("%w: %w", agentrunner.ErrLabelTaken, err)
	case errors.As(err, &notReady):
		// Only the trust_directory dialog is answered: gx raised it by
		// launching in a new directory. Any other dialog is not ours to answer.
		if rule := MatchedRuleID(explain, pane); rule != "trust_directory" {
			return 0, fmt.Errorf("%w: %s blocked on dialog %q gx did not raise", agentrunner.ErrNotReady, name, rule)
		}
		if err := sendKeys(pane, "enter"); err != nil {
			return 0, fmt.Errorf("dismissing %s's trust_directory dialog: %w", name, err)
		}
		return StartResume, nil
	}
	return 0, err
}
