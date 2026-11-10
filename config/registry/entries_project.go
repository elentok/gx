package registry

// projectEntries lists the keys that exist only in project.json. The project
// evolve switch and max-agents get distinct Keys so they do not collide with
// the global entries of the same project key path.
func projectEntries() []Entry {
	p := SectionProject
	return []Entry{
		{Key: Key("name"), Type: TypeString, Section: p, ProjectOnly: true, ReadOnly: true,
			Description: "The project's name."},
		{Key: Key("repo"), Type: TypeString, Section: p, ProjectOnly: true, ReadOnly: true,
			Description: "The repository directory of the project."},
		{Key: Key("vcs"), Type: TypeString, Section: p, ProjectOnly: true, ReadOnly: true,
			Description: "The version control system of the project. none means the project has no repository."},
		{Key: Key("trunk"), Type: TypeString, Section: p, ProjectOnly: true,
			Description: "The branch that finished work lands on."},
		{Key: Key("landing"), Type: TypeEnum, Enum: LandingChoices, Section: p, ProjectOnly: true,
			Description: "When tickets that wait on a blocker are released: on-landed once it landed on trunk, on-done once it is done."},
		{Key: Key("auto-ff-merge"), Type: TypeBool, Section: p, ProjectOnly: true,
			Description: "Fast-forward merge finished work into trunk automatically."},
		{Key: Key("delivery"), Type: TypeEnum, Enum: DeliveryChoices, Section: p, ProjectOnly: true,
			Description: "How finished epics reach trunk: merge fast-forwards them, pr opens a pull request. With pr, dependents are released once the pull request merged."},
		{Key: Key("auto-pr"), Type: TypeBool, Section: p, ProjectOnly: true,
			Description: "Open the pull request automatically when an epic finishes in a pr project."},
		{Key: Key("project", "max-agents"), ProjectKey: Key("max-agents"), GlobalKey: Key("execution-queue", "max-agents"),
			Label: "Max live agents for this project", Override: OverrideCap,
			Type: TypeNumber, Min: minOf(1), Integer: true, Section: SectionExecutionQueue, ProjectOnly: true,
			Description: "The most agents this project runs at the same time, within the global cap."},
		{Key: Key("project", "evolve"), ProjectKey: Key("evolve"), GlobalKey: Key("evolve"),
			Label: "Evolve for this project", Override: OverrideFollowsGlobal,
			Type: TypeBool, Section: SectionEvolveRecovery, ProjectOnly: true,
			Description: "Turn evolve off for this project. Leaving it unset follows the global setting."},
		{Key: Key("checks"), Type: TypeChecks, Section: SectionChecks, ProjectOnly: true,
			Description: "The commands gx check runs for this project."},
	}
}

// LandingChoices repeats the landing values config validates against.
var LandingChoices = []string{"on-landed", "on-done"}

// DeliveryChoices repeats the delivery values config understands.
var DeliveryChoices = []string{"merge", "pr"}
