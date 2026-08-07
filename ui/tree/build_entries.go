package tree

import (
	"path"
	"sort"
	"strings"
)

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

type pathLeaf[T any] struct {
	Path  string
	Value T
}

type pathNode[T any] struct {
	name     string
	path     string
	children map[string]*pathNode[T]
	leaf     *pathLeaf[T]
}

// BuildEntriesFromPaths flattens a flat list of path-bearing values into a
// depth-annotated row list, inferring directory structure from '/'-separated
// path segments and sorting directories before files (alphabetically within
// each). A chain of single-child directories collapses into one entry whose
// DisplayName joins the chain (e.g. "a/b/c"). Every directory entry's Leaves
// field holds every leaf value nested beneath it, collected recursively.
// Every node whose ID (its path) is not present (as true) in collapsed is
// expanded.
func BuildEntriesFromPaths[T any](values []T, pathFn func(T) string, collapsed map[string]bool) []Entry[T] {
	leaves := make([]pathLeaf[T], 0, len(values))
	for i := range values {
		leaves = append(leaves, pathLeaf[T]{
			Path:  pathFn(values[i]),
			Value: values[i],
		})
	}
	return buildPathEntries(leaves, collapsed)
}

func buildPathEntries[T any](leaves []pathLeaf[T], collapsed map[string]bool) []Entry[T] {
	root := &pathNode[T]{children: map[string]*pathNode[T]{}}
	for i := range leaves {
		parts := strings.Split(leaves[i].Path, "/")
		cur := root
		for j := 0; j < len(parts); j++ {
			name := parts[j]
			p := path.Join(parts[:j+1]...)
			next, ok := cur.children[name]
			if !ok {
				next = &pathNode[T]{name: name, path: p, children: map[string]*pathNode[T]{}}
				cur.children[name] = next
			}
			cur = next
		}
		copyLeaf := leaves[i]
		cur.leaf = &copyLeaf
	}

	var entries []Entry[T]
	appendPathEntries(root, "", 0, collapsed, &entries)
	return entries
}

func appendPathEntries[T any](cur *pathNode[T], parentPath string, depth int, collapsed map[string]bool, entries *[]Entry[T]) {
	for _, child := range sortedPathChildren(cur) {
		isDir := len(child.children) > 0
		if !isDir {
			if child.leaf == nil {
				continue
			}
			*entries = append(*entries, Entry[T]{
				ID:          child.path,
				ParentID:    parentPath,
				Depth:       depth,
				DisplayName: child.name,
				Expanded:    true,
				Value:       child.leaf.Value,
			})
			continue
		}

		displayName, dir := collapsedPathDirChain(child)
		expanded := !collapsed[dir.path]
		*entries = append(*entries, Entry[T]{
			ID:          dir.path,
			ParentID:    parentPath,
			Depth:       depth,
			DisplayName: displayName,
			HasChildren: true,
			Expanded:    expanded,
			Leaves:      collectPathLeaves(dir),
		})
		if expanded {
			appendPathEntries(dir, dir.path, depth+1, collapsed, entries)
		}
	}
}

func collapsedPathDirChain[T any](cur *pathNode[T]) (string, *pathNode[T]) {
	parts := []string{cur.name}
	for len(cur.children) == 1 && cur.leaf == nil {
		next := onlyPathChild(cur)
		if next == nil || len(next.children) == 0 {
			break
		}
		parts = append(parts, next.name)
		cur = next
	}
	return path.Join(parts...), cur
}

func onlyPathChild[T any](cur *pathNode[T]) *pathNode[T] {
	for _, child := range cur.children {
		return child
	}
	return nil
}

func collectPathLeaves[T any](cur *pathNode[T]) []T {
	var leaves []T
	if cur.leaf != nil {
		leaves = append(leaves, cur.leaf.Value)
	}
	for _, child := range sortedPathChildren(cur) {
		leaves = append(leaves, collectPathLeaves(child)...)
	}
	return leaves
}

func sortedPathChildren[T any](cur *pathNode[T]) []*pathNode[T] {
	children := make([]*pathNode[T], 0, len(cur.children))
	for _, child := range cur.children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		iDir := len(children[i].children) > 0
		jDir := len(children[j].children) > 0
		if iDir != jDir {
			return iDir
		}
		return children[i].name < children[j].name
	})
	return children
}
