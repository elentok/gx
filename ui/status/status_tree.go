package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/explorer"
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

type aggregateStatus struct {
	hasAny        bool
	hasStaged     bool
	hasUnstaged   bool
	onlyUntracked bool
}

func buildStatusEntries(files []git.StageFileStatus, collapsed map[string]bool) []statusEntry {
	leaves := make([]explorer.FileTreeLeaf[git.StageFileStatus], 0, len(files))
	for i := range files {
		leaves = append(leaves, explorer.FileTreeLeaf[git.StageFileStatus]{
			Path:  files[i].Path,
			Value: files[i],
		})
	}
	rows := explorer.BuildFileTreeRows(leaves, collapsed)
	out := make([]statusEntry, 0, len(rows))
	for _, row := range rows {
		entry := statusEntry{
			Path:        row.Path,
			ParentPath:  row.ParentPath,
			Depth:       row.Depth,
			DisplayName: row.DisplayName,
			Expanded:    row.Expanded,
		}
		if row.Kind == explorer.FileTreeRowDir {
			entry.Kind = statusEntryDir
			agg := aggregateStatusFiles(row.Leaves)
			entry.HasStaged = agg.hasStaged
			entry.HasUnstaged = agg.hasUnstaged
			entry.HasOnlyUntracked = agg.onlyUntracked
		} else {
			entry.Kind = statusEntryFile
			entry.File = row.Value
			entry.HasStaged = row.Value.HasStagedChanges()
			entry.HasUnstaged = row.Value.HasUnstagedChanges()
			entry.HasOnlyUntracked = row.Value.IsUntracked()
		}
		out = append(out, entry)
	}
	return out
}

func aggregateStatusFiles(files []git.StageFileStatus) aggregateStatus {
	agg := aggregateStatus{}
	for _, file := range files {
		childAgg := aggregateStatus{
			hasAny:        true,
			hasStaged:     file.HasStagedChanges(),
			hasUnstaged:   file.HasUnstagedChanges(),
			onlyUntracked: file.IsUntracked(),
		}
		if !agg.hasAny {
			agg = childAgg
			continue
		}
		agg.hasStaged = agg.hasStaged || childAgg.hasStaged
		agg.hasUnstaged = agg.hasUnstaged || childAgg.hasUnstaged
		agg.onlyUntracked = agg.onlyUntracked && childAgg.onlyUntracked
	}
	return agg
}
