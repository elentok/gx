package skills

import "sort"

// SourceClashes maps every skill name that two or more sources share to the
// sorted list of sources holding it. The input is source -> skill names, so the
// result never depends on the order sources were installed in.
func SourceClashes(sources map[string][]string) map[string][]string {
	holders := map[string]map[string]bool{}
	for source, names := range sources {
		for _, name := range names {
			if holders[name] == nil {
				holders[name] = map[string]bool{}
			}
			holders[name][source] = true
		}
	}
	clashes := map[string][]string{}
	for name, set := range holders {
		if len(set) < 2 {
			continue
		}
		for source := range set {
			clashes[name] = append(clashes[name], source)
		}
		sort.Strings(clashes[name])
	}
	return clashes
}
