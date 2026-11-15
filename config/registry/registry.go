// Package registry is the one list of settings keys. It is a leaf package so
// that config can import it; it must not import config.
package registry

import "sort"

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
	// Category, Section and Group place the entry on the settings page.
	Category Category
	Section  Section
	Group    string
	// Tier and Column place a key of a table group (Models) in its table.
	Tier   Tier
	Column Column
	Enum   []string // allowed values of a TypeEnum
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

// Place is where the entry sits on the page.
func (e Entry) Place() Place { return Place{e.Category, e.Section, e.Group} }

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

// New builds a registry with entries sorted stably by category, section and
// group in tree order, keeping the given order within a group.
func New(entries ...Entry) *Registry {
	out := append([]Entry(nil), entries...)
	pos := func(e Entry) int {
		if i, ok := placeOrder[e.Place()]; ok {
			return i
		}
		return len(placeOrder)
	}
	sort.SliceStable(out, func(i, j int) bool { return pos(out[i]) < pos(out[j]) })
	return &Registry{entries: out}
}

// All returns every entry in page order.
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

// ProjectSection is one section of a project's view. Entries are in page
// order, so a group with no overridable key never appears.
type ProjectSection struct {
	Category Category
	Section  Section
	Entries  []Entry
}

// ProjectView groups the entries that apply to a project by section, in tree
// order: Project first, Checks last. A section or category with no applicable
// key is left out.
func (r *Registry) ProjectView() []ProjectSection {
	var out []ProjectSection
	for _, e := range r.entries {
		if !e.AppliesToProject() {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Section == e.Section {
			out[n-1].Entries = append(out[n-1].Entries, e)
			continue
		}
		out = append(out, ProjectSection{Category: e.Category, Section: e.Section, Entries: []Entry{e}})
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
