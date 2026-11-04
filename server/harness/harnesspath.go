package harness

import (
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
	agentharness "github.com/elentok/gx/harness"
)

// DefaultLookPath looks program up over env's PATH, never the process env.
func DefaultLookPath(env []string) func(string) (string, error) {
	return func(program string) (string, error) { return brief.LookPath(env, program) }
}

// MissingProgram is the program a launch of kind runs and whether it is off
// the server's PATH. An unregistered kind is not missing: its own refusal
// names it.
func (h *Harness) MissingProgram(kind string) (program string, missing bool) {
	hr, err := agentharness.Lookup(agentrunner.Kind(kind))
	if err != nil {
		return "", false
	}
	program = hr.Launch(agentharness.LaunchOptions{Env: h.cfg.Env}).Name
	_, err = h.cfg.LookPath(program)
	return program, err != nil
}

// harnessMissingStem is the one message stem of every missing-harness block:
// the program and what chose the harness. Empty when the program is found.
func (h *Harness) harnessMissingStem(kind, from string) string {
	program, missing := h.MissingProgram(kind)
	if !missing {
		return ""
	}
	return fmt.Sprintf("%s is not on PATH (from %s)", program, from)
}

// DefaultHarnessSource names where project's Default harness comes from.
func (h *Harness) DefaultHarnessSource(project string) string {
	if dir, err := h.projects.Dir(project); err == nil {
		if pf, err := config.ReadProjectFile(dir); err == nil && pf.Agents != nil && pf.Agents.DefaultHarness != "" {
			return "the project default"
		}
	}
	return "the global default"
}

// QueueMissingReason is why a ticket resolved to rh cannot be claimed because
// its harness is not on PATH; empty when it is found.
func (h *Harness) QueueMissingReason(project string, rh ResolvedHarness) string {
	from := map[HarnessSource]string{
		HarnessFromFrontmatter: "the ticket's agent",
		HarnessFromQueueItem:   "the queue item",
		HarnessFromEarlierRun:  "the ticket's earlier run",
	}[rh.Source]
	if rh.Source == HarnessFromDefault {
		from = h.DefaultHarnessSource(project)
	}
	stem := h.harnessMissingStem(string(rh.Harness), from)
	if stem == "" {
		return ""
	}
	if rh.Source == HarnessFromFrontmatter {
		return stem + ". Install it, or edit agent: in the ticket"
	}
	return stem + ". Install it, or requeue with --agent <other>"
}

// InteractiveMissingReason is the refusal message of an interactive launch of
// kind whose program is not on PATH; empty when it is found.
func (h *Harness) InteractiveMissingReason(kind, from string) string {
	stem := h.harnessMissingStem(kind, from)
	if stem == "" {
		return ""
	}
	return stem + ". Install it or change the default harness"
}
