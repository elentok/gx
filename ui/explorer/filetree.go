package explorer

import (
	"path"
	"sort"
	"strings"
)

type FileTreeRowKind int

const (
	FileTreeRowFile FileTreeRowKind = iota
	FileTreeRowDir
)

type FileTreeLeaf[T any] struct {
	Path  string
	Value T
}

type FileTreeRow[T any] struct {
	Kind        FileTreeRowKind
	Path        string
	ParentPath  string
	Depth       int
	DisplayName string
	Expanded    bool
	Value       T
	Leaves      []T
}

type fileTreeNode[T any] struct {
	name     string
	path     string
	children map[string]*fileTreeNode[T]
	leaf     *FileTreeLeaf[T]
}

func BuildFileTreeRows[T any](leaves []FileTreeLeaf[T], collapsed map[string]bool) []FileTreeRow[T] {
	root := &fileTreeNode[T]{children: map[string]*fileTreeNode[T]{}}
	for i := range leaves {
		parts := strings.Split(leaves[i].Path, "/")
		cur := root
		for j := 0; j < len(parts); j++ {
			name := parts[j]
			p := path.Join(parts[:j+1]...)
			next, ok := cur.children[name]
			if !ok {
				next = &fileTreeNode[T]{name: name, path: p, children: map[string]*fileTreeNode[T]{}}
				cur.children[name] = next
			}
			cur = next
		}
		copy := leaves[i]
		cur.leaf = &copy
	}

	var rows []FileTreeRow[T]
	appendFileTreeRows(root, "", 0, collapsed, &rows)
	return rows
}

func appendFileTreeRows[T any](node *fileTreeNode[T], parentPath string, depth int, collapsed map[string]bool, rows *[]FileTreeRow[T]) {
	for _, child := range sortedFileTreeChildren(node) {
		isDir := len(child.children) > 0
		if !isDir {
			if child.leaf == nil {
				continue
			}
			*rows = append(*rows, FileTreeRow[T]{
				Kind:        FileTreeRowFile,
				Path:        child.path,
				ParentPath:  parentPath,
				Depth:       depth,
				DisplayName: child.name,
				Expanded:    true,
				Value:       child.leaf.Value,
			})
			continue
		}

		displayName, dir := collapsedFileTreeDirChain(child)
		expanded := !collapsed[dir.path]
		*rows = append(*rows, FileTreeRow[T]{
			Kind:        FileTreeRowDir,
			Path:        dir.path,
			ParentPath:  parentPath,
			Depth:       depth,
			DisplayName: displayName,
			Expanded:    expanded,
			Leaves:      collectFileTreeLeaves(dir),
		})
		if expanded {
			appendFileTreeRows(dir, dir.path, depth+1, collapsed, rows)
		}
	}
}

func collapsedFileTreeDirChain[T any](node *fileTreeNode[T]) (string, *fileTreeNode[T]) {
	parts := []string{node.name}
	cur := node
	for len(cur.children) == 1 && cur.leaf == nil {
		next := onlyFileTreeChild(cur)
		if next == nil || len(next.children) == 0 {
			break
		}
		parts = append(parts, next.name)
		cur = next
	}
	return path.Join(parts...), cur
}

func onlyFileTreeChild[T any](node *fileTreeNode[T]) *fileTreeNode[T] {
	for _, child := range node.children {
		return child
	}
	return nil
}

func collectFileTreeLeaves[T any](node *fileTreeNode[T]) []T {
	var leaves []T
	if node.leaf != nil {
		leaves = append(leaves, node.leaf.Value)
	}
	for _, child := range sortedFileTreeChildren(node) {
		leaves = append(leaves, collectFileTreeLeaves(child)...)
	}
	return leaves
}

func sortedFileTreeChildren[T any](node *fileTreeNode[T]) []*fileTreeNode[T] {
	children := make([]*fileTreeNode[T], 0, len(node.children))
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

func FileTreeParentIndex[T any](rows []FileTreeRow[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(rows) {
		return 0, false
	}
	parent := strings.TrimSpace(rows[selected].ParentPath)
	if parent == "" || parent == rows[selected].Path {
		return 0, false
	}
	for i, row := range rows {
		if row.Kind == FileTreeRowDir && row.Path == parent {
			return i, true
		}
	}
	return 0, false
}

func FileTreeAdjacentFileIndex[T any](rows []FileTreeRow[T], selected, delta int) (int, bool) {
	if delta == 0 || len(rows) == 0 {
		return 0, false
	}
	idx := selected
	for {
		idx += delta
		if idx < 0 || idx >= len(rows) {
			return 0, false
		}
		if rows[idx].Kind == FileTreeRowFile {
			return idx, true
		}
	}
}

func FileTreeCollapseSelectedDir[T any](rows []FileTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != FileTreeRowDir || !row.Expanded {
		return false
	}
	collapsed[row.Path] = true
	return true
}

func FileTreeExpandSelectedDir[T any](rows []FileTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != FileTreeRowDir || row.Expanded {
		return false
	}
	delete(collapsed, row.Path)
	return true
}

func FileTreeToggleDirOnEnter[T any](rows []FileTreeRow[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(rows) {
		return false
	}
	row := rows[selected]
	if row.Kind != FileTreeRowDir {
		return false
	}
	if row.Expanded {
		collapsed[row.Path] = true
	} else {
		delete(collapsed, row.Path)
	}
	return true
}
