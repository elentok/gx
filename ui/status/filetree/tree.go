package filetree

import (
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/tree"
)

type EntryKind int

const (
	EntryFile EntryKind = iota
	EntryDir
)

type Entry struct {
	Kind             EntryKind
	Path             string
	ParentPath       string
	Depth            int
	DisplayName      string
	File             stage.StageFileStatus
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

func buildEntries(files []stage.StageFileStatus, collapsed map[string]bool) []Entry {
	entries, _ := buildEntriesAndRows(files, collapsed)
	return entries
}

func buildEntriesAndRows(files []stage.StageFileStatus, collapsed map[string]bool) ([]Entry, []tree.Entry[stage.StageFileStatus]) {
	entries := tree.BuildEntriesFromPaths(files, func(file stage.StageFileStatus) string {
		return file.Path
	}, collapsed)
	out := make([]Entry, 0, len(entries))
	for _, row := range entries {
		out = append(out, entryFromRow(row))
	}
	return out, entries
}

func aggregateStatusFiles(files []stage.StageFileStatus) aggregateStatus {
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

func entryFromRow(row tree.Entry[stage.StageFileStatus]) Entry {
	entry := Entry{
		Path:        row.ID,
		ParentPath:  row.ParentID,
		Depth:       row.Depth,
		DisplayName: row.DisplayName,
		Expanded:    row.Expanded,
	}
	if row.HasChildren {
		entry.Kind = EntryDir
		agg := aggregateStatusFiles(row.Leaves)
		entry.HasStaged = agg.hasStaged
		entry.HasUnstaged = agg.hasUnstaged
		entry.HasOnlyUntracked = agg.onlyUntracked
		return entry
	}
	entry.Kind = EntryFile
	entry.File = row.Value
	entry.HasStaged = row.Value.HasStagedChanges()
	entry.HasUnstaged = row.Value.HasUnstagedChanges()
	entry.HasOnlyUntracked = row.Value.IsUntracked()
	return entry
}
