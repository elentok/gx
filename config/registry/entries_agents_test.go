package registry

import (
	"strings"
	"testing"
)

func TestAgentsEntries(t *testing.T) {
	r := New(agentsEntries()...)
	type want struct {
		key       KeyPath
		typ       ValueType
		section   Section
		projectOv bool
		restart   bool
		risky     bool
	}
	a, ev, ts := SectionAgents, SectionEvolveRecovery, SectionTicketStore
	cases := []want{
		{key: Key("agents", "default-harness"), typ: TypeEnum, section: a, projectOv: true},
		{key: Key("agents", "restore-sessions-on-start"), typ: TypeBool, section: a},
		{key: Key("agents", "banners", "enabled"), typ: TypeBool, section: a},
		{key: Key("agents", "sounds", "enabled"), typ: TypeBool, section: a},
		{key: Key("agents", "sounds", "done"), typ: TypeString, section: a},
		{key: Key("agents", "sounds", "attention"), typ: TypeString, section: a},
		{key: Key("agents", "claude-args"), typ: TypeStringList, section: a},
		{key: Key("agents", "fix-agent-remote-control"), typ: TypeBool, section: a},
		{key: Key("agent-runner"), typ: TypeEnum, section: a, restart: true},
		{key: Key("runner-wait"), typ: TypeDuration, section: a, restart: true},
		{key: Key("skills", "ci-fix"), typ: TypeStringList, section: SectionPullRequests, projectOv: true},
		{key: Key("skills", "pr-text"), typ: TypeString, section: SectionPullRequests, projectOv: true},
		{key: Key("pr", "ci-fix-attempts-per-check"), typ: TypeNumber, section: SectionPullRequests},
		{key: Key("pr", "ci-fix-attempts-per-pr"), typ: TypeNumber, section: SectionPullRequests},
		{key: Key("pr", "no-checks-grace"), typ: TypeDuration, section: SectionPullRequests},
		{key: Key("evolve"), typ: TypeEnum, section: ev},
		{key: Key("recovery", "enabled"), typ: TypeBool, section: ev},
		{key: Key("recovery", "disabled"), typ: TypeStringList, section: ev},
		{key: Key("recovery", "follow-ups"), typ: TypeString, section: ev},
		{key: Key("recovery", "notify-hold"), typ: TypeDuration, section: ev},
		{key: Key("ticket-store", "path"), typ: TypeString, section: ts, restart: true, risky: true},
		{key: Key("ticket-store", "commit-debounce"), typ: TypeNumber, section: ts},
		{key: Key("ticket-store", "push-remote"), typ: TypeString, section: ts},
	}
	for _, agent := range []string{"claude", "codex"} {
		for _, tier := range [][]string{nil, {"light"}, {"strongest"}} {
			for _, knob := range []string{"model", "effort"} {
				parts := append(append([]string{"agents", agent}, tier...), knob)
				cases = append(cases, want{key: Key(parts...), typ: TypeString, section: SectionModels, projectOv: true})
			}
		}
	}
	if got := len(r.All()); got != len(cases) {
		t.Fatalf("entries = %d, want %d", got, len(cases))
	}
	for _, c := range cases {
		e, ok := r.Lookup(c.key)
		if !ok {
			t.Errorf("%v: missing", c.key)
			continue
		}
		if e.Type != c.typ || e.Section != c.section || e.ProjectOverridable != c.projectOv ||
			e.Restart != c.restart || e.Risky != c.risky || e.ProjectOnly {
			t.Errorf("%v: got %+v", c.key, e)
		}
		if e.Description == "" || e.Risky != (e.ConfirmText != "") {
			t.Errorf("%v: description empty or risky and confirm text disagree", c.key)
		}
		if (e.Type == TypeEnum) != (len(e.Enum) > 0) {
			t.Errorf("%v: enum values and type disagree", c.key)
		}
	}
	if len(Key("evolve.deep-read-cap")) != 1 {
		t.Errorf("evolve.deep-read-cap must be one segment")
	}
	for _, k := range []KeyPath{Key("agents", "leader-key"), Key("agents", "focus-sidebar-key")} {
		if _, ok := r.Lookup(k); ok {
			t.Errorf("%v must have no entry", k)
		}
	}
	if e, _ := r.Lookup(Key("ticket-store", "path")); e.ConfirmText == "" ||
		!strings.Contains(e.ConfirmText, "not moved") {
		t.Errorf("ticket-store.path confirm must say old data is not moved: %q", e.ConfirmText)
	}
}
