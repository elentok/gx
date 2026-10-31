package server

import (
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
)

// VerdictHarnessMissing is the explain verdict for a queued ticket whose
// resolved harness is not on PATH.
const VerdictHarnessMissing = "harness missing"

// defaultLookPath looks program up over env's PATH, never the process env.
func defaultLookPath(env []string) func(string) (string, error) {
	return func(program string) (string, error) { return brief.LookPath(env, program) }
}

// missingProgram is the program a launch of kind runs and whether it is off
// the server's PATH. An unregistered kind is not missing: its own refusal
// names it.
func (s *Server) missingProgram(kind string) (program string, missing bool) {
	h, err := harness.Lookup(agentrunner.Kind(kind))
	if err != nil {
		return "", false
	}
	program = h.Launch(harness.LaunchOptions{Env: s.cfg.Env}).Name
	_, err = s.cfg.LookPath(program)
	return program, err != nil
}

// harnessMissingStem is the one message stem of every missing-harness block:
// the program and what chose the harness. Empty when the program is found.
func (s *Server) harnessMissingStem(kind, from string) string {
	program, missing := s.missingProgram(kind)
	if !missing {
		return ""
	}
	return fmt.Sprintf("%s is not on PATH (from %s)", program, from)
}

// defaultHarnessSource names where project's Default harness comes from.
func (s *Server) defaultHarnessSource(project string) string {
	if dir, err := s.projectDir(project); err == nil {
		if pf, err := config.ReadProjectFile(dir); err == nil && pf.Agents != nil && pf.Agents.DefaultHarness != "" {
			return "the project default"
		}
	}
	return "the global default"
}

// queueMissingReason is why a ticket resolved to rh cannot be claimed because
// its harness is not on PATH; empty when it is found.
func (s *Server) queueMissingReason(project string, rh ResolvedHarness) string {
	from := map[HarnessSource]string{
		HarnessFromFrontmatter: "the ticket's agent",
		HarnessFromQueueItem:   "the queue item",
		HarnessFromEarlierRun:  "the ticket's earlier run",
	}[rh.Source]
	if rh.Source == HarnessFromDefault {
		from = s.defaultHarnessSource(project)
	}
	stem := s.harnessMissingStem(string(rh.Harness), from)
	if stem == "" {
		return ""
	}
	if rh.Source == HarnessFromFrontmatter {
		return stem + ". Install it, or edit agent: in the ticket"
	}
	return stem + ". Install it, or requeue with --agent <other>"
}

// interactiveMissingReason is the refusal message of an interactive launch of
// kind whose program is not on PATH; empty when it is found.
func (s *Server) interactiveMissingReason(kind, from string) string {
	stem := s.harnessMissingStem(kind, from)
	if stem == "" {
		return ""
	}
	return stem + ". Install it or change the default harness"
}
