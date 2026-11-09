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
	SectionAgents
	SectionEvolveRecovery
	SectionTicketStore
	SectionProject
	SectionChecks
)

var sectionNames = [...]string{
	"Execution queue", "Budget", "Notifications", "Server", "Agents",
	"Evolve and recovery", "Ticket store", "Project", "Checks",
}

func (s Section) String() string {
	if s < 0 || int(s) >= len(sectionNames) {
		return "unknown"
	}
	return sectionNames[s]
}

// IsGlobal reports whether the section is one of the seven global sections.
func (s Section) IsGlobal() bool { return s >= SectionExecutionQueue && s <= SectionTicketStore }

// Sections returns every section in order.
func Sections() []Section {
	out := make([]Section, len(sectionNames))
	for i := range out {
		out[i] = Section(i)
	}
	return out
}

// Entry describes one setting. The registry holds no default.
type Entry struct {
	Key         KeyPath
	Type        ValueType
	Min         *float64 // a number's minimum
	Integer     bool     // a number must be whole
	Description string
	Section     Section
	Enum        []string // allowed values of a TypeEnum
	Restart     bool     // startup-only: takes effect after a restart
	Risky       bool
	ConfirmText string // shown before saving a risky entry

	// ProjectOverridable: a project may override this global key.
	// ProjectOnly: the key exists only in a project config.
	ProjectOverridable bool
	ProjectOnly        bool
	// ProjectKey is the project's own key path where it differs from Key.
	ProjectKey KeyPath
}

// WriteOnly reports whether the value is never read back (secrets).
func (e Entry) WriteOnly() bool { return e.Type == TypeSecret }

// AppliesToProject reports whether the entry shows in a project's view.
func (e Entry) AppliesToProject() bool { return e.ProjectOverridable || e.ProjectOnly }

// Exclusion names a config key deliberately left out of the registry.
type Exclusion struct {
	Key    KeyPath
	Reason string
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
