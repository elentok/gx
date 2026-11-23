package registry

import "strings"

// Category, Section and Group place an entry on the settings page. The tree
// below is the only owner of their names and order.
type (
	Category string
	Section  string
)

const (
	CategoryProject       Category = "Project"
	CategoryAgents        Category = "Agents"
	CategoryRalphLoop     Category = "Ralph loop"
	CategoryNotifications Category = "Notifications"
	CategoryMentalHealth  Category = "Mental health"
	CategoryWebUI         Category = "Web UI"
	CategorySystem        Category = "System"
	CategoryChecks        Category = "Checks"
)

const (
	SectionProject        Section = "Project"
	SectionAgents         Section = "Agents"
	SectionModels         Section = "Models"
	SectionExecutionQueue Section = "Execution queue"
	SectionBudget         Section = "Budget"
	SectionPullRequests   Section = "Pull requests"
	SectionEvolveRecovery Section = "Evolve and recovery"
	SectionNotifications  Section = "Notifications"
	SectionMentalHealth   Section = "Mental health"
	SectionAccess         Section = "Access"
	SectionKeybindings    Section = "Keybindings"
	SectionDisplayDev     Section = "Display and development"
	SectionServer         Section = "Server"
	SectionTicketStore    Section = "Ticket store"
	SectionDiskSpace      Section = "Disk space"
	SectionWatchdogs      Section = "Watchdogs"
	SectionChecks         Section = "Checks"
)

// String returns the section's display name.
func (s Section) String() string { return string(s) }

// Slug is the section name lower-cased with dashes.
func (s Section) Slug() string { return strings.ReplaceAll(strings.ToLower(string(s)), " ", "-") }

// Tier and Column place a Models key in its table.
type (
	Tier   string
	Column string
)

const (
	TierLight     Tier = "light"
	TierStandard  Tier = "standard" // the key without a tier prefix
	TierStrongest Tier = "strongest"

	ColumnModel  Column = "model"
	ColumnEffort Column = "effort"
)

// Place is where an entry sits on the page.
type Place struct {
	Category Category
	Section  Section
	Group    string
}

// GroupDef is one group of a section. Table groups render as a table of
// Tier rows and Column columns instead of a list.
type GroupDef struct {
	Name  string
	Table bool
}

// SectionDef is one section and its groups, in order.
type SectionDef struct {
	Name   Section
	Groups []GroupDef
}

// CategoryDef is one category and its sections, in order.
type CategoryDef struct {
	Name     Category
	Sections []SectionDef
}

func groups(names ...string) []GroupDef {
	out := make([]GroupDef, len(names))
	for i, n := range names {
		out[i] = GroupDef{Name: n}
	}
	return out
}

// tree is the full page tree. Project leads and Checks closes; both hold only
// project-only keys, so the global view shows neither. Account is not here:
// the web adds it.
var tree = []CategoryDef{
	{CategoryProject, []SectionDef{
		{SectionProject, groups("Repository", "Delivery")},
	}},
	{CategoryAgents, []SectionDef{
		{SectionAgents, groups("Defaults", "Sessions", "Launch", "Runner")},
		{SectionModels, []GroupDef{{Name: "Claude", Table: true}, {Name: "Codex", Table: true}}},
	}},
	{CategoryRalphLoop, []SectionDef{
		{SectionExecutionQueue, groups("Agent limits", "Waiting and stalled agents")},
		{SectionBudget, groups("Spend limits", "Alerts")},
		{SectionPullRequests, groups("Skills", "CI fix limits", "Waiting")},
		{SectionEvolveRecovery, groups("Evolve", "Recovery")},
	}},
	{CategoryNotifications, []SectionDef{
		{SectionNotifications, groups("Desktop", "Telegram", "Slack", "Web push")},
	}},
	{CategoryMentalHealth, []SectionDef{
		{SectionMentalHealth, groups("General", "Bedtime", "Break")},
	}},
	{CategoryWebUI, []SectionDef{
		{SectionAccess, groups("Network", "Sign-in")},
		{SectionKeybindings, groups("Leader key", "Editors")},
		{SectionDisplayDev, groups("Display", "Development")},
	}},
	{CategorySystem, []SectionDef{
		{SectionServer, groups("Runtime", "Logs")},
		{SectionTicketStore, groups("Location", "Git sync")},
		{SectionDiskSpace, groups("Thresholds", "Cleanup")},
		{SectionWatchdogs, groups("CPU", "Process")},
	}},
	{CategoryChecks, []SectionDef{
		{SectionChecks, groups("Checks")},
	}},
}

// Tree returns the page tree: categories, their sections and groups in order.
func Tree() []CategoryDef {
	out := make([]CategoryDef, len(tree))
	for i, c := range tree {
		out[i] = CategoryDef{Name: c.Name, Sections: append([]SectionDef(nil), c.Sections...)}
	}
	return out
}

// placeOrder maps each placement in the tree to its position in the flat leaf
// order. A placement missing from the map sorts last; the guard test names it.
var placeOrder = func() map[Place]int {
	m := map[Place]int{}
	for _, c := range tree {
		for _, s := range c.Sections {
			for _, g := range s.Groups {
				m[Place{c.Name, s.Name, g.Name}] = len(m)
			}
		}
	}
	return m
}()

// RetiredSlugs maps a section slug that no longer exists to the slug of the
// section that now holds its keys.
func RetiredSlugs() map[string]string {
	return map[string]string{
		"web-ui":           SectionAccess.Slug(),
		"cpu-watchdog":     SectionWatchdogs.Slug(),
		"process-watchdog": SectionWatchdogs.Slug(),
	}
}
