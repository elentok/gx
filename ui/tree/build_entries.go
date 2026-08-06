package tree

// IDFunc returns a stable, unique identifier for a value, used as the
// collapse-state key and to link child rows back to their parent.
type IDFunc[T any] func(T) string

// ChildrenFunc looks up a value's children, in caller-defined order. Order is
// preserved as-is in the flattened output — the package does not sort.
type ChildrenFunc[T any] func(T) []T

// BuildEntriesFromValues flattens a tree of values into a depth-annotated row
// list, expanding every node whose ID is not present (as true) in collapsed.
// Traversal order matches the order values/children are returned in.
func BuildEntriesFromValues[T any](roots []T, idFn IDFunc[T], childrenFn ChildrenFunc[T], collapsed map[string]bool) []Entry[T] {
	var entries []Entry[T]
	appendEntries(roots, "", 0, idFn, childrenFn, collapsed, &entries)
	return entries
}

func appendEntries[T any](values []T, parentID string, depth int, idFn IDFunc[T], childrenFn ChildrenFunc[T], collapsed map[string]bool, entries *[]Entry[T]) {
	for _, v := range values {
		id := idFn(v)
		children := childrenFn(v)
		hasChildren := len(children) > 0
		expanded := hasChildren && !collapsed[id]
		*entries = append(*entries, Entry[T]{
			ID:          id,
			ParentID:    parentID,
			Depth:       depth,
			Value:       v,
			HasChildren: hasChildren,
			Expanded:    expanded,
		})
		if expanded {
			appendEntries(children, id, depth+1, idFn, childrenFn, collapsed, entries)
		}
	}
}
