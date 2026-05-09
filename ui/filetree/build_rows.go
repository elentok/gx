package filetree

import (
	"path"
	"sort"
	"strings"
)

type RowKind int

const (
	RowFile RowKind = iota
	RowDir
)

type Leaf[T any] struct {
	Path  string
	Value T
}

type Row[T any] struct {
	Kind        RowKind
	Path        string
	ParentPath  string
	Depth       int
	DisplayName string
	Expanded    bool
	Value       T
	Leaves      []T
}

type node[T any] struct {
	name     string
	path     string
	children map[string]*node[T]
	leaf     *Leaf[T]
}

func BuildRows[T any](leaves []Leaf[T], collapsed map[string]bool) []Row[T] {
	root := &node[T]{children: map[string]*node[T]{}}
	for i := range leaves {
		parts := strings.Split(leaves[i].Path, "/")
		cur := root
		for j := 0; j < len(parts); j++ {
			name := parts[j]
			p := path.Join(parts[:j+1]...)
			next, ok := cur.children[name]
			if !ok {
				next = &node[T]{name: name, path: p, children: map[string]*node[T]{}}
				cur.children[name] = next
			}
			cur = next
		}
		copyLeaf := leaves[i]
		cur.leaf = &copyLeaf
	}

	var rows []Row[T]
	appendRows(root, "", 0, collapsed, &rows)
	return rows
}

func appendRows[T any](cur *node[T], parentPath string, depth int, collapsed map[string]bool, rows *[]Row[T]) {
	for _, child := range sortedChildren(cur) {
		isDir := len(child.children) > 0
		if !isDir {
			if child.leaf == nil {
				continue
			}
			*rows = append(*rows, Row[T]{
				Kind:        RowFile,
				Path:        child.path,
				ParentPath:  parentPath,
				Depth:       depth,
				DisplayName: child.name,
				Expanded:    true,
				Value:       child.leaf.Value,
			})
			continue
		}

		displayName, dir := collapsedDirChain(child)
		expanded := !collapsed[dir.path]
		*rows = append(*rows, Row[T]{
			Kind:        RowDir,
			Path:        dir.path,
			ParentPath:  parentPath,
			Depth:       depth,
			DisplayName: displayName,
			Expanded:    expanded,
			Leaves:      collectLeaves(dir),
		})
		if expanded {
			appendRows(dir, dir.path, depth+1, collapsed, rows)
		}
	}
}

func collapsedDirChain[T any](cur *node[T]) (string, *node[T]) {
	parts := []string{cur.name}
	for len(cur.children) == 1 && cur.leaf == nil {
		next := onlyChild(cur)
		if next == nil || len(next.children) == 0 {
			break
		}
		parts = append(parts, next.name)
		cur = next
	}
	return path.Join(parts...), cur
}

func onlyChild[T any](cur *node[T]) *node[T] {
	for _, child := range cur.children {
		return child
	}
	return nil
}

func collectLeaves[T any](cur *node[T]) []T {
	var leaves []T
	if cur.leaf != nil {
		leaves = append(leaves, cur.leaf.Value)
	}
	for _, child := range sortedChildren(cur) {
		leaves = append(leaves, collectLeaves(child)...)
	}
	return leaves
}

func sortedChildren[T any](cur *node[T]) []*node[T] {
	children := make([]*node[T], 0, len(cur.children))
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
