package commit

import "strings"

type commitSearchMode int

const (
	commitSearchModeNone commitSearchMode = iota
	commitSearchModeInput
)

type commitSearchDismissPolicy int

const (
	commitSearchDismissAlwaysClear commitSearchDismissPolicy = iota
	commitSearchDismissKeepResultsUnlessEmptyOrNoMatches
)

func commitSearchEnter() commitSearchMode {
	return commitSearchModeInput
}

func commitSearchDismiss(query *string, cursor *int, total int, policy commitSearchDismissPolicy) (mode commitSearchMode, cleared bool) {
	mode = commitSearchModeNone
	if cursor != nil && *cursor < 0 {
		*cursor = 0
	}
	switch policy {
	case commitSearchDismissAlwaysClear:
		if query != nil {
			*query = ""
		}
		if cursor != nil {
			*cursor = 0
		}
		return mode, true
	case commitSearchDismissKeepResultsUnlessEmptyOrNoMatches:
		if strings.TrimSpace(derefString(query)) == "" || total == 0 {
			if query != nil {
				*query = ""
			}
			if cursor != nil {
				*cursor = 0
			}
			return mode, true
		}
	}
	return mode, false
}

func commitSearchCanNavigate(query string, total int) bool {
	return strings.TrimSpace(query) != "" && total > 0
}

func commitSearchCursorNext(cursor *int, total int) bool {
	if cursor == nil || total <= 0 {
		return false
	}
	if *cursor < total-1 {
		*cursor = *cursor + 1
		return true
	}
	return false
}

func commitSearchCursorPrev(cursor *int, total int) bool {
	if cursor == nil || total <= 0 {
		return false
	}
	if *cursor > 0 {
		*cursor = *cursor - 1
		return true
	}
	return false
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
