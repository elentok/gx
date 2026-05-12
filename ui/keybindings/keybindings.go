package keybindings

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// BindingID is a type-safe identifier for a keybinding. Each model defines its
// own constants using this type.
type BindingID string

// Binding describes a single keybinding — its key sequence, help metadata, and categories.
type Binding struct {
	ID         BindingID // dispatch identifier, e.g. "goto-log"
	Seq        []string  // key sequence: ["g","l"] for chord, ["j"] for single key
	Categories []string  // help sections this binding appears in
	Title      string    // description shown in help, e.g. "goto log"
	Display    string    // optional key display override (defaults to Seq joined with "/")
}

// Keys returns the display string for this binding's key sequence.
func (b Binding) Keys() string {
	if b.Display != "" {
		return b.Display
	}
	return strings.Join(b.Seq, "/")
}

// Manager is a value type — embed it in the bubbletea model so prefix state is
// copied correctly on each update.
type Manager struct {
	bindings []Binding
	prefix   []string // accumulated key sequence in progress
}

// New creates a Manager with the given bindings.
func New(bindings []Binding) Manager {
	return Manager{bindings: bindings}
}

// Process feeds a key press into the manager and returns:
//   - match != nil, consumed=true  → sequence complete, dispatch on match.ID
//   - match == nil, consumed=true  → chord in progress, call ChordHints() for status bar
//   - match == nil, consumed=false → key not registered, fall through to child delegation
func (m *Manager) Process(msg tea.KeyPressMsg) (match *Binding, consumed bool) {
	return m.process(normalizeKey(msg))
}

// normalizeKey converts a KeyPressMsg to the canonical string the Manager matches
// against. It handles bubbletea terminal inconsistencies, such as some terminals
// sending lowercase 'g' with ModShift instead of 'G'.
func normalizeKey(msg tea.KeyPressMsg) string {
	key := msg.String()
	if key == "g" && msg.Mod&tea.ModShift != 0 {
		return "G"
	}
	return key
}

func (m *Manager) process(key string) (match *Binding, consumed bool) {
	candidate := append(m.prefix, key)

	// Check for an exact match.
	for i := range m.bindings {
		b := &m.bindings[i]
		if seqEqual(b.Seq, candidate) {
			m.prefix = nil
			return b, true
		}
	}

	// Check if candidate is a valid prefix of any binding.
	for _, b := range m.bindings {
		if hasPrefix(b.Seq, candidate) {
			m.prefix = candidate
			return nil, true
		}
	}

	// No match and no valid prefix — cancel any in-progress chord.
	m.prefix = nil
	return nil, false
}

// ChordHints returns hints for all bindings that extend the current internal
// prefix, so the caller can render them in the status bar.
func (m Manager) ChordHints() []ChordHint {
	if len(m.prefix) == 0 {
		return nil
	}
	var hints []ChordHint
	for _, b := range m.bindings {
		if len(b.Seq) > len(m.prefix) && seqEqual(b.Seq[:len(m.prefix)], m.prefix) {
			hints = append(hints, ChordHint{
				Key:  b.Seq[len(m.prefix)],
				Desc: b.Title,
			})
		}
	}
	return hints
}

// ChordHint is a single chord completion hint for display in the status bar.
type ChordHint struct {
	Key  string
	Desc string
}

// Bindings returns all registered bindings in registration order.
func (m Manager) Bindings() []Binding {
	return m.bindings
}

// Reset clears the accumulated prefix. Call this whenever a modal opens so the
// manager doesn't carry dirty state into the next key event.
func (m *Manager) Reset() {
	m.prefix = nil
}

// Prefix returns the current in-progress key sequence (for testing/debugging).
func (m Manager) Prefix() []string {
	return m.prefix
}

func seqEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasPrefix(seq, prefix []string) bool {
	if len(prefix) >= len(seq) {
		return false
	}
	return seqEqual(seq[:len(prefix)], prefix)
}
