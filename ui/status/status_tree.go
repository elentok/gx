package stage

import (
	"path"
	"sort"
	"strings"

	"github.com/elentok/gx/git"
)

type statusEntryKind int

const (
	statusEntryFile statusEntryKind = iota
	statusEntryDir
)

type statusEntry struct {
	Kind             statusEntryKind
	Path             string
	ParentPath       string
	Depth            int
	DisplayName      string
	File             git.StageFileStatus
	HasStaged        bool
	HasUnstaged      bool
	HasOnlyUntracked bool
	Expanded         bool
}

type statusNode struct {
	name     string
	path     string
	children map[string]*statusNode
	file     *git.StageFileStatus
}

type aggregateStatus struct {
	hasAny        bool
	hasStaged     bool
	hasUnstaged   bool
	onlyUntracked bool
}

func buildStatusEntries(files []git.StageFileStatus, collapsed map[string]bool) []statusEntry {
	root := &statusNode{children: map[string]*statusNode{}}
	for i := range files {
		parts := strings.Split(files[i].Path, "/")
		cur := root
		for j := 0; j < len(parts); j++ {
			name := parts[j]
			p := path.Join(parts[:j+1]...)
			next, ok := cur.children[name]
			if !ok {
				next = &statusNode{name: name, path: p, children: map[string]*statusNode{}}
				cur.children[name] = next
			}
			cur = next
		}
		copy := files[i]
		cur.file = &copy
	}

	var out []statusEntry
	appendVisibleEntries(root, "", 0, collapsed, &out)
	return out
}

func appendVisibleEntries(node *statusNode, parentPath string, depth int, collapsed map[string]bool, out *[]statusEntry) {
	for _, child := range sortedChildren(node) {
		isDir := len(child.children) > 0
		if !isDir {
			if child.file == nil {
				continue
			}
			*out = append(*out, statusEntry{
				Kind:             statusEntryFile,
				Path:             child.path,
				ParentPath:       parentPath,
				Depth:            depth,
				DisplayName:      child.name,
				File:             *child.file,
				HasStaged:        child.file.HasStagedChanges(),
				HasUnstaged:      child.file.HasUnstagedChanges(),
				HasOnlyUntracked: child.file.IsUntracked(),
			})
			continue
		}

		displayName, dir := collapsedDirChain(child)
		agg := aggregateNode(dir)
		expanded := !collapsed[dir.path]
		*out = append(*out, statusEntry{
			Kind:             statusEntryDir,
			Path:             dir.path,
			ParentPath:       parentPath,
			Depth:            depth,
			DisplayName:      displayName,
			HasStaged:        agg.hasStaged,
			HasUnstaged:      agg.hasUnstaged,
			HasOnlyUntracked: agg.onlyUntracked,
			Expanded:         expanded,
		})
		if expanded {
			appendVisibleEntries(dir, dir.path, depth+1, collapsed, out)
		}
	}
}

func collapsedDirChain(node *statusNode) (string, *statusNode) {
	parts := []string{node.name}
	cur := node
	for len(cur.children) == 1 && cur.file == nil {
		next := onlyChild(cur)
		if next == nil || len(next.children) == 0 {
			break
		}
		parts = append(parts, next.name)
		cur = next
	}
	return path.Join(parts...), cur
}

func onlyChild(node *statusNode) *statusNode {
	for _, child := range node.children {
		return child
	}
	return nil
}

func aggregateNode(node *statusNode) aggregateStatus {
	agg := aggregateStatus{}
	if node.file != nil {
		agg.hasAny = true
		agg.hasStaged = node.file.HasStagedChanges()
		agg.hasUnstaged = node.file.HasUnstagedChanges()
		agg.onlyUntracked = node.file.IsUntracked()
	}
	for _, child := range node.children {
		childAgg := aggregateNode(child)
		if !childAgg.hasAny {
			continue
		}
		if !agg.hasAny {
			agg = childAgg
			continue
		}
		agg.hasAny = true
		agg.hasStaged = agg.hasStaged || childAgg.hasStaged
		agg.hasUnstaged = agg.hasUnstaged || childAgg.hasUnstaged
		agg.onlyUntracked = agg.onlyUntracked && childAgg.onlyUntracked
	}
	return agg
}

func sortedChildren(node *statusNode) []*statusNode {
	children := make([]*statusNode, 0, len(node.children))
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
