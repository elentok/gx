package registry

import "strings"

// KeyPath addresses a setting as a slice of segments. It is not a dotted
// string: the flat key "evolve.deep-read-cap" is ONE segment in config.json,
// so splitting on dots would misread it.
type KeyPath []string

// Key builds a key path from its segments.
func Key(segments ...string) KeyPath { return KeyPath(segments) }

// String is the display form: segments joined with dots.
func (k KeyPath) String() string { return strings.Join(k, ".") }

// Equal reports whether both paths have the same segments.
func (k KeyPath) Equal(o KeyPath) bool {
	if len(k) != len(o) {
		return false
	}
	for i := range k {
		if k[i] != o[i] {
			return false
		}
	}
	return true
}
