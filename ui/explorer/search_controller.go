package explorer

import "strings"

type SearchMode int

const (
	SearchModeNone SearchMode = iota
	SearchModeInput
)

type SearchDismissPolicy int

const (
	SearchDismissAlwaysClear SearchDismissPolicy = iota
	SearchDismissKeepResultsUnlessEmptyOrNoMatches
)

func SearchEnter() SearchMode {
	return SearchModeInput
}

func SearchExitInput() SearchMode {
	return SearchModeNone
}

func SearchDismiss(query *string, cursor *int, total int, policy SearchDismissPolicy) (mode SearchMode, cleared bool) {
	mode = SearchModeNone
	if cursor != nil && *cursor < 0 {
		*cursor = 0
	}
	switch policy {
	case SearchDismissAlwaysClear:
		if query != nil {
			*query = ""
		}
		if cursor != nil {
			*cursor = 0
		}
		return mode, true
	case SearchDismissKeepResultsUnlessEmptyOrNoMatches:
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

func SearchCanNavigate(query string, total int) bool {
	return strings.TrimSpace(query) != "" && total > 0
}

func SearchCursorNext(cursor *int, total int) bool {
	if cursor == nil || total <= 0 {
		return false
	}
	if *cursor < total-1 {
		*cursor = *cursor + 1
		return true
	}
	return false
}

func SearchCursorPrev(cursor *int, total int) bool {
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
