package commit

import (
	"path"
	"sort"
	"strings"
)

type commitTreeRowKind int

const (
	commitTreeRowFile commitTreeRowKind = iota
	commitTreeRowDir
)

type commitTreeLeaf[T any] struct {
	Path  string
	Value T
}

type commitTreeRow[T any] struct {
	Kind        commitTreeRowKind
	Path        string
	ParentPath  string
	Depth       int
	DisplayName string
	Expanded    bool
	Value       T
	Leaves      []T
}

type commitTreeNode[T any] struct {
	name     string
	path     string
	children map[string]*commitTreeNode[T]
	leaf     *commitTreeLeaf[T]
}

func buildCommitTreeRows[T any](leaves []commitTreeLeaf[T], collapsed map[string]bool) []commitTreeRow[T] {
	root := &commitTreeNode[T]{children: map[string]*commitTreeNode[T]{}}
	for i := range leaves {
		parts := strings.Split(leaves[i].Path, "/")
		cur := root
		for j := 0; j < len(parts); j++ {
			name := parts[j]
			p := path.Join(parts[:j+1]...)
			next, ok := cur.children[name]
			if !ok {
				next = &commitTreeNode[T]{name: name, path: p, children: map[string]*commitTreeNode[T]{}}
				cur.children[name] = next
			}
			cur = next
		}
		copy := leaves[i]
		cur.leaf = &copy
	}

	var rows []commitTreeRow[T]
	appendCommitTreeRows(root, "", 0, collapsed, &rows)
	return rows
}

func appendCommitTreeRows[T any](node *commitTreeNode[T], parentPath string, depth int, collapsed map[string]bool, rows *[]commitTreeRow[T]) {
	for _, child := range sortedCommitTreeChildren(node) {
		isDir := len(child.children) > 0
		if !isDir {
			if child.leaf == nil {
				continue
			}
			*rows = append(*rows, commitTreeRow[T]{
				Kind:        commitTreeRowFile,
				Path:        child.path,
				ParentPath:  parentPath,
				Depth:       depth,
				DisplayName: child.name,
				Expanded:    true,
				Value:       child.leaf.Value,
			})
			continue
		}

		displayName, dir := collapsedCommitTreeDirChain(child)
		expanded := !collapsed[dir.path]
		*rows = append(*rows, commitTreeRow[T]{
			Kind:        commitTreeRowDir,
			Path:        dir.path,
			ParentPath:  parentPath,
			Depth:       depth,
			DisplayName: displayName,
			Expanded:    expanded,
			Leaves:      collectCommitTreeLeaves(dir),
		})
		if expanded {
			appendCommitTreeRows(dir, dir.path, depth+1, collapsed, rows)
		}
	}
}

func collapsedCommitTreeDirChain[T any](node *commitTreeNode[T]) (string, *commitTreeNode[T]) {
	parts := []string{node.name}
	cur := node
	for len(cur.children) == 1 && cur.leaf == nil {
		next := onlyCommitTreeChild(cur)
		if next == nil || len(next.children) == 0 {
			break
		}
		parts = append(parts, next.name)
		cur = next
	}
	return path.Join(parts...), cur
}

func onlyCommitTreeChild[T any](node *commitTreeNode[T]) *commitTreeNode[T] {
	for _, child := range node.children {
		return child
	}
	return nil
}

func collectCommitTreeLeaves[T any](node *commitTreeNode[T]) []T {
	var leaves []T
	if node.leaf != nil {
		leaves = append(leaves, node.leaf.Value)
	}
	for _, child := range sortedCommitTreeChildren(node) {
		leaves = append(leaves, collectCommitTreeLeaves(child)...)
	}
	return leaves
}

func sortedCommitTreeChildren[T any](node *commitTreeNode[T]) []*commitTreeNode[T] {
	children := make([]*commitTreeNode[T], 0, len(node.children))
	for _, child := range node.children {
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

func commitTreeParentIndex[T any](rows []commitTreeRow[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(rows) {
		return 0, false
	}
	parent := strings.TrimSpace(rows[selected].ParentPath)
	if parent == "" || parent == rows[selected].Path {
		return 0, false
	}
	for i, row := range rows {
		if row.Kind == commitTreeRowDir && row.Path == parent {
			return i, true
		}
	}
	return 0, false
}

func commitTreeAdjacentFileIndex[T any](rows []commitTreeRow[T], selected, delta int) (int, bool) {
	if delta == 0 || len(rows) == 0 {
		return 0, false
	}
	idx := selected
	for {
		idx += delta
		if idx < 0 || idx >= len(rows) {
			return 0, false
		}
		if rows[idx].Kind == commitTreeRowFile {
			return idx, true
		}
	}
}

func commitTreeCollapseSelectedDir[T any](rows []commitTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != commitTreeRowDir || !row.Expanded {
		return false
	}
	collapsed[row.Path] = true
	return true
}

func commitTreeExpandSelectedDir[T any](rows []commitTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != commitTreeRowDir || row.Expanded {
		return false
	}
	delete(collapsed, row.Path)
	return true
}

func commitTreeToggleDirOnEnter[T any](rows []commitTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != commitTreeRowDir {
		return false
	}
	if row.Expanded {
		collapsed[row.Path] = true
	} else {
		delete(collapsed, row.Path)
	}
	return true
}
