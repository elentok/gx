package registry

import "github.com/elentok/gx/agentrunner"

// HarnessChoices and EvolveChoices repeat the values config validates
// against: this package cannot import config. config's tests pin them to the
// loader's constants.
var (
	HarnessChoices = []string{"claude", "codex"}
	EvolveChoices  = []string{"off", "maintainer", "contributor"}
)

func runnerChoices() []string {
	return []string{string(agentrunner.ChoiceAuto),
		string(agentrunner.ChoiceHeadless), string(agentrunner.ChoicePTY)}
}

// agentsEntries lists the Agents, Models, Pull requests, Evolve and recovery
// and Ticket store keys. leader-key and focus-sidebar-key are left out on
// purpose.
func agentsEntries() []Entry {
	var out []Entry
	add := func(p Place, e Entry) {
		e.Category, e.Section, e.Group = p.Category, p.Section, p.Group
		e.Label = labelFor(e)
		out = append(out, e)
	}
	agents := func(g string) Place { return Place{CategoryAgents, SectionAgents, g} }
	ralph := func(s Section, g string) Place { return Place{CategoryRalphLoop, s, g} }

	add(agents("Defaults"), Entry{Key: Key("agents", "default-harness"), Type: TypeEnum, Enum: HarnessChoices, ProjectOverridable: true,
		Description: "The coding agent that new sessions start with."})
	for _, agent := range []struct{ id, group string }{{"claude", "Claude"}, {"codex", "Codex"}} {
		tiers := []struct {
			tier Tier
			path []string
			desc string
		}{
			{TierStandard, nil, "the standard tier"},
			{TierLight, []string{"light"}, "the light tier"},
			{TierStrongest, []string{"strongest"}, "the strongest tier"},
		}
		for _, t := range tiers {
			base := append([]string{"agents", agent.id}, t.path...)
			place := Place{CategoryAgents, SectionModels, agent.group}
			add(place, Entry{Key: Key(append(base[:len(base):len(base)], "model")...), Type: TypeString, ProjectOverridable: true,
				Tier: t.tier, Column: ColumnModel,
				Description: "The model " + agent.id + " uses for " + t.desc + ". Empty keeps the agent's own setting."})
			add(place, Entry{Key: Key(append(base[:len(base):len(base)], "effort")...), Type: TypeString, ProjectOverridable: true,
				Tier: t.tier, Column: ColumnEffort,
				Description: "The effort level " + agent.id + " uses for " + t.desc + ". Empty keeps the agent's own setting."})
		}
	}
	add(agents("Sessions"), Entry{Key: Key("agents", "restore-sessions-on-start"), Type: TypeBool,
		Description: "Resume every interactive session that was running when the server last stopped. Default: false."})
	add(agents("Sessions"), Entry{Key: Key("agents", "fix-agent-remote-control"), Type: TypeBool,
		Description: "Start Claude fix-agent sessions with remote control on."})
	add(agents("Launch"), Entry{Key: Key("agents", "claude-args"), Type: TypeStringList,
		Description: "Extra command-line arguments passed to every Claude session."})
	add(agents("Launch"), Entry{Key: Key("agents", "writable-directories"), Type: TypeStringList, ProjectOverridable: true, Override: OverrideAdd,
		Description: "Additional directories agents may write. Project directories are added to the global list. Supports ~/ paths; relative paths use the agent's working directory. Applies on launch or resume. Default: ~/Library/pnpm."})
	add(agents("Runner"), Entry{Key: Key("agent-runner"), Type: TypeEnum, Enum: runnerChoices(), Restart: true,
		Description: "How gx runs agents: auto picks for you, or choose headless or pty."})
	add(agents("Runner"), Entry{Key: Key("runner-wait"), Type: TypeDuration, Restart: true,
		Description: "How long an agent runner waits on its guards before giving up."})

	prs := SectionPullRequests
	add(ralph(prs, "Skills"), Entry{Key: Key("skills", "ci-fix"), Type: TypeStringList, ProjectOverridable: true,
		Description: "The skills a ci-fix ticket runs, in order. Empty uses the bundled gx-ci-fix."})
	add(ralph(prs, "Skills"), Entry{Key: Key("skills", "pr-text"), Type: TypeString, ProjectOverridable: true,
		Description: "The skill that writes the pull request title and body. Empty uses the bundled gx-pr-text."})
	add(ralph(prs, "CI fix limits"), Entry{Key: Key("pr", "ci-fix-attempts-per-check"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "The most ci-fix tickets gx files for one failing check. Default: 3."})
	add(ralph(prs, "CI fix limits"), Entry{Key: Key("pr", "ci-fix-attempts-per-pr"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "The most ci-fix tickets gx files for one pull request. Default: 10."})
	add(ralph(prs, "Waiting"), Entry{Key: Key("pr", "no-checks-grace"), Type: TypeDuration,
		Description: "How long a pull request with no checks waits before it counts as green. Default: 2m."})

	ev := SectionEvolveRecovery
	add(ralph(ev, "Evolve"), Entry{Key: Key("evolve"), Type: TypeEnum, Enum: EvolveChoices,
		Description: "What gx does when an epic completes: off, maintainer or contributor."})
	add(ralph(ev, "Recovery"), Entry{Key: Key("recovery", "enabled"), Type: TypeBool,
		Description: "Let gx try to recover tickets that get stuck."})
	add(ralph(ev, "Recovery"), Entry{Key: Key("recovery", "disabled"), Type: TypeStringList,
		Description: "Recovery steps that stay off."})
	add(ralph(ev, "Recovery"), Entry{Key: Key("recovery", "follow-ups"), Type: TypeString,
		Description: "Where recovery files follow-ups."})
	add(ralph(ev, "Recovery"), Entry{Key: Key("recovery", "notify-hold"), Type: TypeDuration,
		Description: "How long recovery holds a notification back before sending it."})

	store := func(g string) Place { return Place{CategorySystem, SectionTicketStore, g} }
	add(store("Location"), Entry{Key: Key("ticket-store", "path"), Type: TypeString, Restart: true, Risky: true,
		ConfirmText: "Your existing tickets are not moved. gx will look for tickets at the new path and you will see an empty store until you move the old data yourself.",
		Description: "The directory that holds your tickets."})
	add(store("Git sync"), Entry{Key: Key("ticket-store", "commit-debounce"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "Seconds the ticket store waits after a change before it commits."})
	add(store("Git sync"), Entry{Key: Key("ticket-store", "push-remote"), Type: TypeString,
		Description: "A git remote the ticket store is pushed to after each commit. Empty means no push."})
	return out
}
