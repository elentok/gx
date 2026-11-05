package filetree

import (
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
)

type paneIcons struct {
	folderClosed string
	folderOpen   string
	fileModified string
	fileNew      string
	fileDeleted  string
	fileRenamed  string
	fileSymlink  string
	partial      string
	staged       string
}

func paneIconsFor(useNerdFontIcons bool) paneIcons {
	shared := ui.Icons(useNerdFontIcons)
	return paneIcons{
		folderClosed: shared.FolderClosed,
		folderOpen:   shared.FolderOpen,
		fileModified: shared.FileModified,
		fileNew:      shared.FileAdded,
		fileDeleted:  shared.FileDeleted,
		fileRenamed:  shared.FileRenamed,
		fileSymlink:  shared.FileSymlink,
		partial:      shared.Partial,
		staged:       shared.Staged,
	}
}

func entryColor(entry Entry) string {
	if entry.Kind == EntryFile && isDeletedFileStatus(entry.File) {
		return "#a6adc8"
	}
	if entry.Kind == EntryFile && entry.File.IsRenamed() {
		return "#89b4fa"
	}
	if entry.HasStaged && entry.HasUnstaged {
		return "#fab387"
	}
	if entry.HasStaged {
		return "#a6e3a1"
	}
	return "#cdd6f4"
}

func entryMeta(entry Entry, useNerdFontIcons bool, icons paneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return " "
	}
	if entry.Kind == EntryDir {
		return "-"
	}
	return entry.File.XY()
}

func fileIcon(file stage.StageFileStatus, isSymlink bool, icons paneIcons) string {
	if isDeletedFileStatus(file) {
		return icons.fileDeleted
	}
	if file.IsRenamed() {
		return icons.fileRenamed
	}
	if isSymlink {
		return icons.fileSymlink
	}
	if file.IsUntracked() || file.IndexStatus == 'A' {
		return icons.fileNew
	}
	return icons.fileModified
}

func isDeletedFileStatus(file stage.StageFileStatus) bool {
	return file.IndexStatus == 'D' || file.WorktreeCode == 'D'
}
