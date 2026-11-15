package registry

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func tinyRegistry() *Registry {
	return New(
		Entry{Key: Key("server", "port"), Type: TypeNumber, Category: CategorySystem, Section: SectionServer, Group: "Runtime"},
		Entry{Key: Key("evolve.deep-read-cap"), Type: TypeNumber, Category: CategoryRalphLoop, Section: SectionEvolveRecovery, Group: "Evolve", ProjectOverridable: true},
		Entry{Key: Key("queue", "max-agents"), Type: TypeNumber, Category: CategoryRalphLoop, Section: SectionExecutionQueue, Group: "Agent limits",
			ProjectOverridable: true, ProjectKey: Key("max-agents")},
		Entry{Key: Key("checks"), Type: TypeChecks, Category: CategoryChecks, Section: SectionChecks, Group: "Checks", ProjectOnly: true},
		Entry{Key: Key("token"), Type: TypeSecret, Category: CategorySystem, Section: SectionServer, Group: "Runtime"},
	)
}

func TestLookupByKeyPath(t *testing.T) {
	r := tinyRegistry()
	e, ok := r.Lookup(Key("queue", "max-agents"))
	if !ok || e.ProjectKey.String() != "max-agents" {
		t.Fatalf("lookup = %+v, %v", e, ok)
	}
	if _, ok := r.Lookup(Key("queue")); ok {
		t.Fatal("prefix must not match")
	}
}

func TestEntriesInSectionOrder(t *testing.T) {
	var got []Section
	for _, e := range tinyRegistry().All() {
		got = append(got, e.Section)
	}
	want := []Section{SectionExecutionQueue, SectionEvolveRecovery, SectionServer, SectionServer, SectionChecks}
	if len(got) != len(want) {
		t.Fatalf("sections = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sections = %v, want %v", got, want)
		}
	}
	if n := len(tinyRegistry().InSection(SectionServer)); n != 2 {
		t.Fatalf("server entries = %d", n)
	}
	if SectionExecutionQueue.String() != "Execution queue" || SectionEvolveRecovery.Slug() != "evolve-and-recovery" {
		t.Fatal("section name or slug wrong")
	}
}

func TestFlatDottedKeyStaysOneSegment(t *testing.T) {
	k := Key("evolve.deep-read-cap")
	if len(k) != 1 {
		t.Fatalf("segments = %d", len(k))
	}
	if k.String() != "evolve.deep-read-cap" {
		t.Fatalf("String = %q", k.String())
	}
	if _, ok := tinyRegistry().Lookup(k); !ok {
		t.Fatal("flat key not found")
	}
	if _, ok := tinyRegistry().Lookup(Key("evolve", "deep-read-cap")); ok {
		t.Fatal("split key must not match the flat key")
	}
}

func TestProjectViewAndWriteOnly(t *testing.T) {
	r := tinyRegistry()
	proj := r.ForProject()
	if len(proj) != 3 {
		t.Fatalf("project entries = %d", len(proj))
	}
	for _, e := range proj {
		if !e.ProjectOverridable && !e.ProjectOnly {
			t.Fatalf("%v leaked into project view", e.Key)
		}
	}
	tok, _ := r.Lookup(Key("token"))
	port, _ := r.Lookup(Key("server", "port"))
	if !tok.WriteOnly() || port.WriteOnly() {
		t.Fatal("write-only must be exactly secret entries")
	}
}

// placementProblems names every entry whose placement is missing or not in the
// tree, and every category, section or group that holds no entry.
func placementProblems(r *Registry) []string {
	var out []string
	used := map[Place]bool{}
	for _, e := range r.All() {
		switch {
		case e.Category == "" || e.Section == "" || e.Group == "":
			out = append(out, "key "+e.Key.String()+" has no category, section or group")
		case !inTree(e.Place()):
			out = append(out, "key "+e.Key.String()+" is placed outside the tree: "+string(e.Category)+"/"+string(e.Section)+"/"+e.Group)
		}
		used[e.Place()] = true
	}
	for _, c := range Tree() {
		catUsed := false
		for _, s := range c.Sections {
			secUsed := false
			for _, g := range s.Groups {
				if used[Place{c.Name, s.Name, g.Name}] {
					secUsed, catUsed = true, true
				} else {
					out = append(out, "group "+string(c.Name)+"/"+string(s.Name)+"/"+g.Name+" is empty")
				}
			}
			if !secUsed {
				out = append(out, "section "+string(s.Name)+" is empty")
			}
		}
		if !catUsed {
			out = append(out, "category "+string(c.Name)+" is empty")
		}
	}
	return out
}

func inTree(p Place) bool {
	_, ok := placeOrder[p]
	return ok
}

func TestPlacementGuard(t *testing.T) {
	if problems := placementProblems(Default()); len(problems) > 0 {
		t.Fatalf("placement problems:\n%s", strings.Join(problems, "\n"))
	}
}

func TestPlacementGuardNamesTheKey(t *testing.T) {
	good := Place{CategorySystem, SectionServer, "Runtime"}
	for name, bad := range map[string]Entry{
		"no category": {Section: good.Section, Group: good.Group},
		"no section":  {Category: good.Category, Group: good.Group},
		"no group":    {Category: good.Category, Section: good.Section},
		"not in tree": {Category: good.Category, Section: good.Section, Group: "Nowhere"},
	} {
		bad.Key = Key("some", "key")
		got := strings.Join(placementProblems(New(bad)), "\n")
		if !strings.Contains(got, "key some.key") {
			t.Errorf("%s: problems do not name the key: %q", name, got)
		}
	}
	// A registry that fills only some of the tree leaves the rest empty.
	got := strings.Join(placementProblems(New(Entry{Key: Key("k"), Category: good.Category, Section: good.Section, Group: good.Group})), "\n")
	for _, want := range []string{"group System/Server/Logs is empty", "section Budget is empty", "category Agents is empty"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestTreeOrderAndModelsTables(t *testing.T) {
	var cats []Category
	for _, c := range Tree() {
		cats = append(cats, c.Name)
	}
	want := []Category{CategoryProject, CategoryAgents, CategoryRalphLoop, CategoryNotifications, CategoryWebUI, CategorySystem, CategoryChecks}
	if !slices.Equal(cats, want) {
		t.Fatalf("categories = %v, want %v", cats, want)
	}
	for _, c := range Tree() {
		for _, s := range c.Sections {
			for _, g := range s.Groups {
				if isTable := s.Name == SectionModels; g.Table != isTable {
					t.Errorf("%s/%s table = %v, want %v", s.Name, g.Name, g.Table, isTable)
				}
			}
		}
	}
}

func TestRetiredSlugs(t *testing.T) {
	want := map[string]string{"web-ui": "access", "cpu-watchdog": "watchdogs", "process-watchdog": "watchdogs"}
	if got := RetiredSlugs(); !maps.Equal(got, want) {
		t.Fatalf("retired slugs = %v, want %v", got, want)
	}
	for _, c := range Tree() {
		for _, s := range c.Sections {
			if _, retired := RetiredSlugs()[s.Name.Slug()]; retired {
				t.Errorf("section slug %q must not redirect", s.Name.Slug())
			}
		}
	}
}

// TestPlacementPinsTable lists every key's place from the spec's page tree
// table, so a move is a deliberate edit of this test.
func TestPlacementPinsTable(t *testing.T) {
	want := map[string]string{}
	pin := func(place string, keys ...string) {
		for _, k := range keys {
			want[k] = place
		}
	}
	pin("Agents/Agents/Defaults", "agents.default-harness")
	pin("Agents/Agents/Sessions", "agents.restore-sessions-on-start", "agents.fix-agent-remote-control")
	pin("Agents/Agents/Launch", "agents.claude-args", "agents.writable-directories")
	pin("Agents/Agents/Runner", "agent-runner", "runner-wait")
	for _, a := range []string{"claude", "codex"} {
		group := map[string]string{"claude": "Claude", "codex": "Codex"}[a]
		for _, prefix := range []string{"", "light.", "strongest."} {
			pin("Agents/Models/"+group, "agents."+a+"."+prefix+"model", "agents."+a+"."+prefix+"effort")
		}
	}
	pin("Ralph loop/Execution queue/Agent limits", "execution-queue.max-agents", "project.max-agents",
		"execution-queue.max-agents-per-epic", "execution-queue.max-one-off-agents")
	pin("Ralph loop/Execution queue/Waiting and stalled agents", "execution-queue.waiting-slot-grace",
		"execution-queue.max-released-agents", "execution-queue.stalled-nudge-delay")
	pin("Ralph loop/Budget/Spend limits", "budget.soft-limit", "budget.hard-limit")
	pin("Ralph loop/Budget/Alerts", "budget.long-ticket-turns", "subscription.suppress-extra-usage-warning")
	pin("Ralph loop/Pull requests/Skills", "skills.ci-fix", "skills.pr-text")
	pin("Ralph loop/Pull requests/CI fix limits", "pr.ci-fix-attempts-per-check", "pr.ci-fix-attempts-per-pr")
	pin("Ralph loop/Pull requests/Waiting", "pr.no-checks-grace")
	pin("Ralph loop/Evolve and recovery/Evolve", "evolve", "project.evolve")
	pin("Ralph loop/Evolve and recovery/Recovery", "recovery.enabled", "recovery.disabled", "recovery.follow-ups", "recovery.notify-hold")
	pin("Notifications/Notifications/Desktop", "agents.banners.enabled", "agents.sounds.enabled", "agents.sounds.done", "agents.sounds.attention")
	pin("Notifications/Notifications/Telegram", "notifications.telegram.bot-token", "notifications.telegram.chat-id")
	pin("Notifications/Notifications/Slack", "notifications.slack.webhook-url")
	pin("Web UI/Access/Network", "server.web.listen", "server.web.allowed-hosts", "server.web.files.roots")
	pin("Web UI/Access/Sign-in", "server.web.totp")
	pin("Web UI/Keybindings/Leader key", "server.web.leader-key-mac", "server.web.leader-key")
	pin("Web UI/Keybindings/Editors", "server.web.vi-escape")
	pin("Web UI/Display and development/Display", "server.recently-finished")
	pin("Web UI/Display and development/Development", "server.web.dev-frontend", "server.web.dev-dir")
	pin("System/Server/Runtime", "server.watch-config", "server.tab-env")
	pin("System/Server/Logs", "server.log-retention")
	pin("System/Ticket store/Location", "ticket-store.path")
	pin("System/Ticket store/Git sync", "ticket-store.commit-debounce", "ticket-store.push-remote")
	pin("System/Disk space/Thresholds", "disk-space.warn-below", "disk-space.clean-below")
	pin("System/Disk space/Cleanup", "disk-space.cleanup-command")
	pin("System/Watchdogs/CPU", "cpu-watchdog.enabled", "cpu-watchdog.threshold-percent", "cpu-watchdog.sustain", "cpu-watchdog.cooldown")
	pin("System/Watchdogs/Process", "watchdog.max-tree-processes", "watchdog.hard-max-tree-processes", "watchdog.interval", "watchdog.ulimit-headroom")
	pin("Project/Project/Repository", "name", "repo", "vcs")
	pin("Project/Project/Delivery", "trunk", "landing", "delivery", "auto-ff-merge", "auto-pr")
	pin("Checks/Checks/Checks", "checks")

	got := map[string]string{}
	for _, e := range Default().All() {
		got[e.Key.String()] = string(e.Category) + "/" + string(e.Section) + "/" + e.Group
	}
	if !maps.Equal(got, want) {
		for k, v := range got {
			if want[k] != v {
				t.Errorf("%s: placed %q, table says %q", k, v, want[k])
			}
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				t.Errorf("%s: in the table but not in the registry", k)
			}
		}
	}
}
