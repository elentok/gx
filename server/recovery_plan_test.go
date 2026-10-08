package server

import (
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
)

func TestRecoveryPlan_DecideMapsEachCaseToOneAction(t *testing.T) {
	noop := func(recovery.Failure, recovery.Verbs) error { return nil }
	rule := recovery.Entry{ID: "R9", Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Remedy: noop}
	high := recovery.Entry{ID: "R9", Executor: recovery.ExecutorRule, Authority: recovery.AuthorityHigh, Remedy: noop}
	ruleNoRemedy := recovery.Entry{ID: "R9", Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow}
	agent := recovery.Entry{ID: "R9", Executor: recovery.ExecutorAgent}
	person := recovery.Entry{ID: "R9", Executor: recovery.ExecutorPerson}
	recognize := recovery.Entry{ID: "R9", Executor: recovery.ExecutorRecognize}

	park := recovery.Failure{Type: events.NeedsRepair, Kind: events.Spinning}
	blocked := recovery.Failure{Type: events.NeedsAnswer, Kind: events.BlockedPane}
	deadlock := recovery.Failure{Type: events.Deadlocked}
	recovered := []ralphloop.Event{{Type: string(events.RecoveryApplied), Kind: string(events.Spinning), Reason: "R9"}}

	tests := []struct {
		name     string
		f        recovery.Failure
		log      []ralphloop.Event
		entry    recovery.Entry
		matched  bool
		diagnose bool
		ok       bool
		action   recoveryAction
		why      string
	}{
		{name: "unmatched investigates", f: park, ok: true, action: actionInvestigate},
		{name: "unmatched blocked pane gets nothing", f: blocked},
		{name: "rule runs", f: park, entry: rule, matched: true, ok: true, action: actionRunRule},
		{name: "rule with no remedy gets nothing", f: park, entry: ruleNoRemedy, matched: true},
		{name: "high authority proposes", f: park, entry: high, matched: true, ok: true, action: actionPropose},
		{name: "agent investigates", f: park, entry: agent, matched: true, ok: true, action: actionInvestigate},
		{name: "person escalates", f: park, entry: person, matched: true, ok: true, action: actionEscalate, why: "R9 is a person's to handle"},
		{name: "recognize only records", f: park, entry: recognize, matched: true, ok: true, action: actionNone},
		{name: "recognize skips the guard rail", f: park, log: recovered, entry: recognize, matched: true, ok: true, action: actionNone},
		{name: "guard rail escalates a rule", f: park, log: recovered, entry: rule, matched: true, ok: true, action: actionEscalate, why: "recovery R9 for spinning failed: ticket failed again"},
		{name: "guard rail escalates an investigation", f: park, log: recovered, ok: true, action: actionEscalate, why: "recovery R9 for spinning failed: ticket failed again"},
		{name: "diagnosis investigates a matched rule", f: deadlock, entry: rule, matched: true, diagnose: true, ok: true, action: actionInvestigate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := recoveryPlan{log: tt.log, entry: tt.entry, matched: tt.matched}
			ok := plan.decide(tt.f, tt.diagnose)
			if ok != tt.ok {
				t.Fatalf("decide ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if plan.action != tt.action || plan.why != tt.why {
				t.Fatalf("decide = (%v, %q), want (%v, %q)", plan.action, plan.why, tt.action, tt.why)
			}
			if plan.holdsPark() != (tt.action == actionRunRule) {
				t.Fatalf("holdsPark = %v for action %v", plan.holdsPark(), tt.action)
			}
		})
	}
}
