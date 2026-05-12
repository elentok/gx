package filetree

import "strings"

func parentIndex[T any](entries []Entry[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(entries) {
		return 0, false
	}
	parent := strings.TrimSpace(entries[selected].ParentPath)
	if parent == "" || parent == entries[selected].Path {
		return 0, false
	}
	for i, entry := range entries {
		if entry.Kind == EntryDir && entry.Path == parent {
			return i, true
		}
	}
	return 0, false
}

func adjacentFileIndex[T any](entries []Entry[T], selected, delta int) (int, bool) {
	if delta == 0 || len(entries) == 0 {
		return 0, false
	}
	idx := selected
	for {
		idx += delta
		if idx < 0 || idx >= len(entries) {
			return 0, false
		}
		if entries[idx].Kind == EntryFile {
			return idx, true
		}
	}
}

func firstChildIndex[T any](entries []Entry[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(entries) {
		return 0, false
	}
	entry := entries[selected]
	if entry.Kind != EntryDir {
		return 0, false
	}
	for i := selected + 1; i < len(entries); i++ {
		candidate := entries[i]
		if candidate.ParentPath == entry.Path {
			return i, true
		}
		if candidate.Depth <= entry.Depth {
			break
		}
	}
	return 0, false
}

func collapseSelectedDir[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if entry.Kind != EntryDir || !entry.Expanded {
		return false
	}
	collapsed[entry.Path] = true
	return true
}

func expandSelectedDir[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if entry.Kind != EntryDir || entry.Expanded {
		return false
	}
	delete(collapsed, entry.Path)
	return true
}

func toggleDirOnEnter[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if entry.Kind != EntryDir {
		return false
	}
	if entry.Expanded {
		collapsed[entry.Path] = true
	} else {
		delete(collapsed, entry.Path)
	}
	return true
}
