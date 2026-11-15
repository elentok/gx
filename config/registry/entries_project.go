package registry

// projectEntries lists the keys that exist only in project.json. The project
// evolve switch and max-agents get distinct Keys so they do not collide with
// the global entries of the same project key path.
func projectEntries() []Entry {
	repo := Place{CategoryProject, SectionProject, "Repository"}
	delivery := Place{CategoryProject, SectionProject, "Delivery"}
	at := func(p Place, e Entry) Entry {
		e.Category, e.Section, e.Group = p.Category, p.Section, p.Group
		return e
	}
	return []Entry{
		at(repo, Entry{Key: Key("name"), Type: TypeString, ProjectOnly: true, ReadOnly: true, Identity: true,
			Description: "The project's name."}),
		at(repo, Entry{Key: Key("repo"), Type: TypeString, ProjectOnly: true, ReadOnly: true, Identity: true,
			Label:       "Repository",
			Description: "The repository directory of the project."}),
		at(repo, Entry{Key: Key("vcs"), Type: TypeString, ProjectOnly: true, ReadOnly: true, Identity: true,
			Label:       "VCS",
			Description: "The version control system of the project. none means the project has no repository."}),
		at(delivery, Entry{Key: Key("trunk"), Type: TypeString, ProjectOnly: true,
			Description: "The branch that finished work lands on."}),
		at(delivery, Entry{Key: Key("landing"), Type: TypeEnum, Enum: LandingChoices, ProjectOnly: true,
			Description: "When tickets that wait on a blocker are released: on-landed once it landed on trunk, on-done once it is done."}),
		at(delivery, Entry{Key: Key("delivery"), Type: TypeEnum, Enum: DeliveryChoices, ProjectOnly: true,
			Description: "How finished epics reach trunk: merge fast-forwards them, pr opens a pull request. With pr, dependents are released once the pull request merged."}),
		at(delivery, Entry{Key: Key("auto-ff-merge"), Type: TypeBool, ProjectOnly: true,
			Description: "Fast-forward merge finished work into trunk automatically."}),
		at(delivery, Entry{Key: Key("auto-pr"), Type: TypeBool, ProjectOnly: true,
			Description: "Open the pull request automatically when an epic finishes in a pr project."}),
		at(Place{CategoryRalphLoop, SectionExecutionQueue, "Agent limits"}, Entry{
			Key: Key("project", "max-agents"), ProjectKey: Key("max-agents"), GlobalKey: Key("execution-queue", "max-agents"),
			Label: "Max live agents for this project", Override: OverrideCap,
			Type: TypeNumber, Min: minOf(1), Integer: true, ProjectOnly: true,
			Description: "The most agents this project runs at the same time, within the global cap."}),
		at(Place{CategoryRalphLoop, SectionEvolveRecovery, "Evolve"}, Entry{
			Key: Key("project", "evolve"), ProjectKey: Key("evolve"), GlobalKey: Key("evolve"),
			Label: "Evolve for this project", Override: OverrideFollowsGlobal,
			Type: TypeBool, ProjectOnly: true,
			Description: "Turn evolve off for this project. Leaving it unset follows the global setting."}),
		at(Place{CategoryChecks, SectionChecks, "Checks"}, Entry{Key: Key("checks"), Type: TypeChecks, ProjectOnly: true,
			Description: "The commands gx check runs for this project."}),
	}
}

// LandingChoices repeats the landing values config validates against.
var LandingChoices = []string{"on-landed", "on-done"}

// DeliveryChoices repeats the delivery values config understands.
var DeliveryChoices = []string{"merge", "pr"}
