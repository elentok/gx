package tree

func parentIndex[T any](entries []Entry[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(entries) {
		return 0, false
	}
	parentID := entries[selected].ParentID
	if parentID == "" {
		return 0, false
	}
	for i, entry := range entries {
		if entry.ID == parentID {
			return i, true
		}
	}
	return 0, false
}

func adjacentLeafIndex[T any](entries []Entry[T], selected, delta int) (int, bool) {
	if delta == 0 || len(entries) == 0 {
		return 0, false
	}
	idx := selected
	for {
		idx += delta
		if idx < 0 || idx >= len(entries) {
			return 0, false
		}
		if !entries[idx].HasChildren {
			return idx, true
		}
	}
}

func firstChildIndex[T any](entries []Entry[T], selected int) (int, bool) {
	if selected < 0 || selected >= len(entries) {
		return 0, false
	}
	entry := entries[selected]
	if !entry.HasChildren {
		return 0, false
	}
	for i := selected + 1; i < len(entries); i++ {
		candidate := entries[i]
		if candidate.ParentID == entry.ID {
			return i, true
		}
		if candidate.Depth <= entry.Depth {
			break
		}
	}
	return 0, false
}

func collapseSelected[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if !entry.HasChildren || !entry.Expanded {
		return false
	}
	collapsed[entry.ID] = true
	return true
}

// expandSelected records an explicit, durable "user expanded this" entry
// (collapsed[id] = false) rather than deleting the map key — a deleted key is
// indistinguishable from an ID nobody has ever touched, which is invisible
// for a node that defaults to expanded but silently re-collapses a node
// whose caller-declared default is collapsed (see ApplyDefaults) on the next
// rebuild.
func expandSelected[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if !entry.HasChildren || entry.Expanded {
		return false
	}
	collapsed[entry.ID] = false
	return true
}

func toggleOnEnter[T any](entries []Entry[T], collapsed map[string]bool, selected int) bool {
	if selected < 0 || selected >= len(entries) {
		return false
	}
	entry := entries[selected]
	if !entry.HasChildren {
		return false
	}
	collapsed[entry.ID] = entry.Expanded
	return true
}
