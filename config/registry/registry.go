// Package registry is the one list of settings keys. It is a leaf package so
// that config can import it; it must not import config.
package registry

// ValueType is the kind of value a setting holds.
type ValueType int

const (
	TypeBool ValueType = iota
	TypeNumber
	TypeDuration
	TypeSize
	TypeEnum
	TypeString
	TypeStringList
	TypeNumberList
	TypeSecret
	TypeChecks
)

// Section groups entries on the settings page. "Global" is not a section.
type Section int

// Global sections come first, in page order; project-only sections follow.
const (
	SectionExecutionQueue Section = iota
	SectionBudget
	SectionNotifications
	SectionServer
	SectionWebUI
	SectionKeybindings
	SectionCPUWatchdog
	SectionProcessWatchdog
	SectionDiskSpace
	SectionAgents
	SectionModels
	SectionPullRequests
	SectionEvolveRecovery
	SectionTicketStore
	SectionProject
	SectionChecks
)

var sectionNames = [...]string{
	"Execution queue", "Budget", "Notifications", "Server", "Web UI",
	"Keybindings", "CPU watchdog", "Process watchdog", "Disk space", "Agents", "Models",
	"Pull requests", "Evolve and recovery", "Ticket store", "Project", "Checks",
}

func (s Section) String() string {
	if s < 0 || int(s) >= len(sectionNames) {
		return "unknown"
	}
	return sectionNames[s]
}

// IsGlobal reports whether the section is one of the global sections.
func (s Section) IsGlobal() bool { return s >= SectionExecutionQueue && s <= SectionTicketStore }

// Sections returns every section in order.
func Sections() []Section {
	out := make([]Section, len(sectionNames))
	for i := range out {
		out[i] = Section(i)
	}
	return out
}

// OverrideMode is how a project row relates to its global key.
type OverrideMode string

const (
	// OverrideValue (the zero value): the project holds its own value that
	// replaces the global one.
	OverrideValue OverrideMode = ""
	// OverrideCap: the project holds a cap that sits under the global value.
	OverrideCap OverrideMode = "cap"
	// OverrideFollowsGlobal: a bool that is on by following the global
	// setting; the project file holds the key only to turn it off.
	OverrideFollowsGlobal OverrideMode = "follows-global"
	// OverrideAdd appends project values to the global list.
	OverrideAdd OverrideMode = "add"
)

// Entry describes one setting. The registry holds no default.
type Entry struct {
	Key         KeyPath
	Type        ValueType
	Min         *float64 // a number's minimum
	Integer     bool     // a number must be whole
	Description string
	Section     Section
	Enum        []string // allowed values of a TypeEnum
	// CheckString, when set, checks a TypeString value. It returns a short
	// sentence the page shows after "Not saved: ", or nil.
	CheckString func(string) error
	Restart     bool // startup-only: takes effect after a restart
	Risky       bool
	ConfirmText string // shown before saving a risky entry
	// RiskyOffOnly: only turning a bool off (or resetting a set value) is
	// risky; turning it on makes things safer and saves without asking.
	RiskyOffOnly bool
	// Label overrides the row's name where the key's last segment reads badly.
	Label string
	// Override says how a project's row behaves for a key with a GlobalKey.
	Override OverrideMode

	// ProjectOverridable: a project may override this global key.
	// ProjectOnly: the key exists only in a project config.
	ProjectOverridable bool
	ProjectOnly        bool
	// ProjectKey is the project's own key path where it differs from Key.
	ProjectKey KeyPath
	// GlobalKey links a project-only entry to the global key it caps or
	// overrides, for "global cap of X" text.
	GlobalKey KeyPath
	// ReadOnly: shown but never written by the page.
	ReadOnly bool
	// Identity marks the entries that say which project this is; the page
	// shows them as a header, not as rows.
	Identity bool
}

// ProjectPath is the key path in project.json.
func (e Entry) ProjectPath() KeyPath {
	if len(e.ProjectKey) > 0 {
		return e.ProjectKey
	}
	return e.Key
}

// WriteOnly reports whether the value is never read back (secrets).
func (e Entry) WriteOnly() bool { return e.Type == TypeSecret }

// AppliesToProject reports whether the entry shows in a project's view.
func (e Entry) AppliesToProject() bool { return e.ProjectOverridable || e.ProjectOnly }

// Exclusion names a config key deliberately left out of the registry.
type Exclusion struct {
	Key KeyPath
	// Project: the key is excluded from project.json, not config.json.
	Project bool
	Reason  string
}

// Registry is an ordered set of entries.
type Registry struct {
	entries []Entry
}

// New builds a registry with entries sorted stably by section, keeping the
// given order within a section.
func New(entries ...Entry) *Registry {
	var out []Entry
	for _, s := range Sections() {
		for _, e := range entries {
			if e.Section == s {
				out = append(out, e)
			}
		}
	}
	return &Registry{entries: out}
}

// All returns every entry in section order.
func (r *Registry) All() []Entry { return append([]Entry(nil), r.entries...) }

// Lookup finds an entry by key path.
func (r *Registry) Lookup(k KeyPath) (Entry, bool) {
	for _, e := range r.entries {
		if e.Key.Equal(k) {
			return e, true
		}
	}
	return Entry{}, false
}

// InSection returns the entries of one section.
func (r *Registry) InSection(s Section) []Entry {
	var out []Entry
	for _, e := range r.entries {
		if e.Section == s {
			out = append(out, e)
		}
	}
	return out
}

// Default is the registry of every setting.
func Default() *Registry {
	var all []Entry
	// Project entries first: within a section the project's own row leads the project view.
	all = append(all, projectEntries()...)
	all = append(all, coreEntries()...)
	all = append(all, agentsEntries()...)
	return New(all...)
}

// ProjectSection is one section of a project's view.
type ProjectSection struct {
	Section Section
	Entries []Entry
}

// ProjectView groups the entries that apply to a project: Project first, then
// each other section that holds one, Checks last. Notifications never shows.
func (r *Registry) ProjectView() []ProjectSection {
	order := []Section{SectionProject}
	for _, s := range Sections() {
		if s != SectionProject && s != SectionChecks && s != SectionNotifications {
			order = append(order, s)
		}
	}
	order = append(order, SectionChecks)
	var out []ProjectSection
	for _, s := range order {
		var es []Entry
		for _, e := range r.InSection(s) {
			if e.AppliesToProject() {
				es = append(es, e)
			}
		}
		if len(es) > 0 {
			out = append(out, ProjectSection{Section: s, Entries: es})
		}
	}
	return out
}

// ForProject returns the entries that apply to a project.
func (r *Registry) ForProject() []Entry {
	var out []Entry
	for _, e := range r.entries {
		if e.AppliesToProject() {
			out = append(out, e)
		}
	}
	return out
}
