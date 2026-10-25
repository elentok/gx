package brief

import (
	"slices"
	"strings"
)

// splitArgs separates the flags brief may rewrite from the "-args" tail, which
// belongs to the test binary and passes through untouched. The tail keeps its
// "-args" marker.
func splitArgs(args []string) (rest, tail []string) {
	if i := slices.Index(args, "-args"); i >= 0 {
		return args[:i], args[i:]
	}
	return args, nil
}

// stripFlag removes every occurrence of the named flags (given without
// dashes, written with one or two) in "-name" or "-name=value" form. With
// takesValue, a bare "-name" also removes the argument after it. It reports
// whether it removed anything.
func stripFlag(args []string, takesValue bool, names ...string) ([]string, bool) {
	var out []string
	found := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || !slices.Contains(names, name) {
			out = append(out, a)
			continue
		}
		found = true
		if takesValue && !hasValue {
			i++
		}
	}
	return out, found
}
