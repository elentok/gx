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
	return []string{string(agentrunner.ChoiceAuto), string(agentrunner.ChoiceHerdr),
		string(agentrunner.ChoiceHeadless), string(agentrunner.ChoicePTY)}
}

// agentsEntries lists the Agents, Evolve and recovery and Ticket store
// sections. leader-key and focus-sidebar-key are left out on purpose.
func agentsEntries() []Entry {
	var out []Entry
	add := func(s Section, e Entry) {
		e.Section = s
		out = append(out, e)
	}

	a := SectionAgents
	add(a, Entry{Key: Key("agents", "default-harness"), Type: TypeEnum, Enum: HarnessChoices, ProjectOverridable: true,
		Description: "The coding agent that new sessions start with."})
	for _, agent := range []string{"claude", "codex"} {
		tiers := []struct {
			path []string
			desc string
		}{
			{nil, "the standard tier"},
			{[]string{"light"}, "the light tier"},
			{[]string{"strongest"}, "the strongest tier"},
		}
		for _, t := range tiers {
			base := append([]string{"agents", agent}, t.path...)
			add(a, Entry{Key: Key(append(base[:len(base):len(base)], "model")...), Type: TypeString, ProjectOverridable: true,
				Description: "The model " + agent + " uses for " + t.desc + ". Empty keeps the agent's own setting."})
			add(a, Entry{Key: Key(append(base[:len(base):len(base)], "effort")...), Type: TypeString, ProjectOverridable: true,
				Description: "The effort level " + agent + " uses for " + t.desc + ". Empty keeps the agent's own setting."})
		}
	}
	add(a, Entry{Key: Key("agents", "restore-sessions-on-start"), Type: TypeBool,
		Description: "Reopen the agent sessions that were open when gx last stopped."})
	add(a, Entry{Key: Key("agents", "banners", "enabled"), Type: TypeBool,
		Description: "Show an OS banner when an interactive agent session needs you."})
	add(a, Entry{Key: Key("agents", "sounds", "enabled"), Type: TypeBool,
		Description: "Play a sound when an interactive agent session finishes or needs you."})
	add(a, Entry{Key: Key("agents", "sounds", "done"), Type: TypeString,
		Description: "The sound file played when a session finishes, relative to the config directory. Empty uses the built-in sound."})
	add(a, Entry{Key: Key("agents", "sounds", "attention"), Type: TypeString,
		Description: "The sound file played when a session needs you, relative to the config directory. Empty uses the built-in sound."})
	add(a, Entry{Key: Key("agents", "claude-args"), Type: TypeStringList,
		Description: "Extra command-line arguments passed to every Claude session."})
	add(a, Entry{Key: Key("agents", "fix-agent-remote-control"), Type: TypeBool,
		Description: "Start Claude fix-agent sessions with remote control on."})
	add(a, Entry{Key: Key("skills", "implement"), Type: TypeString, ProjectOverridable: true,
		Description: "The skill agents run to implement a ticket."})
	add(a, Entry{Key: Key("skills", "code-review"), Type: TypeStringList, ProjectOverridable: true,
		Description: "The skills that review an epic's code."})
	add(a, Entry{Key: Key("agent-runner"), Type: TypeEnum, Enum: runnerChoices(), Restart: true,
		Description: "How gx runs agents: auto picks for you, or choose herdr, headless or pty."})
	add(a, Entry{Key: Key("runner-wait"), Type: TypeDuration, Restart: true,
		Description: "How long an agent runner waits on its guards before giving up."})

	e := SectionEvolveRecovery
	add(e, Entry{Key: Key("evolve"), Type: TypeEnum, Enum: EvolveChoices,
		Description: "What gx does when an epic completes: off, maintainer or contributor."})
	add(e, Entry{Key: Key("recovery", "enabled"), Type: TypeBool,
		Description: "Let gx try to recover tickets that get stuck."})
	add(e, Entry{Key: Key("recovery", "disabled"), Type: TypeStringList,
		Description: "Recovery steps that stay off."})
	add(e, Entry{Key: Key("recovery", "follow-ups"), Type: TypeString,
		Description: "Where recovery files follow-ups."})
	add(e, Entry{Key: Key("recovery", "notify-hold"), Type: TypeDuration,
		Description: "How long recovery holds a notification back before sending it."})

	t := SectionTicketStore
	add(t, Entry{Key: Key("ticket-store", "path"), Type: TypeString, Restart: true, Risky: true,
		ConfirmText: "Your existing tickets are not moved. gx will look for tickets at the new path and you will see an empty store until you move the old data yourself.",
		Description: "The directory that holds your tickets."})
	add(t, Entry{Key: Key("ticket-store", "commit-debounce"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "Seconds the ticket store waits after a change before it commits."})
	add(t, Entry{Key: Key("ticket-store", "push-remote"), Type: TypeString,
		Description: "A git remote the ticket store is pushed to after each commit. Empty means no push."})
	return out
}
