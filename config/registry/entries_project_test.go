package registry

import "testing"

func TestProjectView(t *testing.T) {
	view := Default().ProjectView()
	var order []Section
	for _, ps := range view {
		order = append(order, ps.Section)
	}
	want := []Section{SectionProject, SectionExecutionQueue, SectionDiskSpace, SectionAgents, SectionModels, SectionPullRequests, SectionEvolveRecovery, SectionChecks}
	if len(order) != len(want) {
		t.Fatalf("sections = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("sections = %v, want %v", order, want)
		}
	}
	paths := map[string]Entry{}
	for _, ps := range view {
		for _, e := range ps.Entries {
			if ps.Section == SectionNotifications {
				t.Errorf("notifications entry in project view: %v", e.Key)
			}
			if e.Key.String() == "evolve.deep-read-cap" {
				t.Error("evolve.deep-read-cap must not be in the project view")
			}
			if len(e.Key) > 1 && e.Key[0] == "agents" && e.Key[1] != "claude" && e.Key[1] != "codex" && e.Key[1] != "default-harness" && e.Key[1] != "writable-directories" {
				t.Errorf("ignored agents sub-key in project view: %v", e.Key)
			}
			paths[e.ProjectPath().String()] = e
		}
	}
	m, ok := paths["max-agents"]
	if !ok || m.GlobalKey.String() != "execution-queue.max-agents" || m.Section != SectionExecutionQueue {
		t.Errorf("max-agents = %+v", m)
	}
	ev := paths["evolve"]
	if ev.Type != TypeBool || ev.Section != SectionEvolveRecovery {
		t.Errorf("project evolve = %+v", ev)
	}
	if m.Label != "Max live agents for this project" || m.Override != OverrideCap {
		t.Errorf("max-agents label/override = %q/%q", m.Label, m.Override)
	}
	if ev.Label != "Evolve for this project" || ev.Override != OverrideFollowsGlobal {
		t.Errorf("evolve label/override = %q/%q", ev.Label, ev.Override)
	}
	if totp, _ := Default().Lookup(Key("server", "web", "totp")); !totp.RiskyOffOnly {
		t.Errorf("server.web.totp must be risky only when turned off")
	}
}

func TestProjectViewLeadsExecutionQueueWithCap(t *testing.T) {
	for _, ps := range Default().ProjectView() {
		if ps.Section == SectionExecutionQueue && ps.Entries[0].Key.String() != "project.max-agents" {
			t.Errorf("first execution-queue row = %v, want project.max-agents", ps.Entries[0].Key)
		}
	}
}

func TestProjectOnlyEntries(t *testing.T) {
	r := Default()
	for _, k := range []string{"name", "repo", "vcs"} {
		e, ok := r.Lookup(Key(k))
		if !ok || !e.ReadOnly || !e.ProjectOnly || e.Section != SectionProject {
			t.Errorf("%s: %+v", k, e)
		}
	}
	for _, k := range []string{"trunk", "landing", "auto-ff-merge", "checks"} {
		e, ok := r.Lookup(Key(k))
		if !ok || e.ReadOnly || !e.ProjectOnly {
			t.Errorf("%s: %+v", k, e)
		}
	}
	if e, _ := r.Lookup(Key("landing")); e.Type != TypeEnum || len(e.Enum) != 2 {
		t.Errorf("landing = %+v", e)
	}
	if e, _ := r.Lookup(Key("checks")); e.Type != TypeChecks || e.Section != SectionChecks {
		t.Errorf("checks = %+v", e)
	}
}

func TestExclusionsHaveReasons(t *testing.T) {
	got := map[string]Exclusion{}
	for _, x := range Exclusions() {
		if x.Reason == "" {
			t.Errorf("%v: empty reason", x.Key)
		}
		got[x.Key.String()] = x
	}
	for _, k := range []string{
		"$schema", "agents.resume-on-start", "notifications",
		"agents.restore-sessions-on-start", "agents.banners", "agents.sounds", "agents.claude-args", "agents.fix-agent-remote-control",
		"use-nerdfont-icons", "image-diffs", "stage-diff-context-lines", "input-modal-bottom", "name-aliases",
		"log", "log.important-refs", "log.hide-refs", "agents.leader-key", "agents.focus-sidebar-key",
		"execution-queue.retry-storm-launches", "execution-queue.spin-cycles", "execution-queue.spin-window",
		"budget.notification-thresholds", "evolve.deep-read-cap",
	} {
		if _, ok := got[k]; !ok {
			t.Errorf("exclusion %s missing", k)
		}
	}
	if x := got["notifications"]; !x.Project {
		t.Error("project notifications exclusion must be project-scoped")
	}
	if _, ok := Default().Lookup(Key("cpu-watchdog", "enabled")); !ok {
		t.Error("cpu-watchdog.enabled must stay an entry")
	}
	r := Default()
	for k, x := range got {
		if _, ok := r.Lookup(x.Key); ok && !x.Project {
			t.Errorf("excluded key %s is also an entry", k)
		}
	}
}

func TestCoverage(t *testing.T) {
	r := Default()
	for _, s := range Sections() {
		if len(r.InSection(s)) == 0 {
			t.Errorf("section %v has no entries", s)
		}
	}
	for _, e := range r.All() {
		if !e.ProjectOverridable {
			continue
		}
		if len(e.ProjectPath()) == 0 || !e.Section.IsGlobal() {
			t.Errorf("%v: no project placement", e.Key)
		}
	}
}
