package diffview

import (
	"fmt"
	"strings"
)

type FocusedYankError string

const (
	FocusedYankErrNoHunk  FocusedYankError = "no hunk selected"
	FocusedYankErrNoLines FocusedYankError = "no lines to yank"
)

func FormatYankLocation(path, loc string) string {
	path = strings.TrimSpace(path)
	loc = strings.TrimSpace(loc)
	if path == "" {
		return ""
	}
	if loc == "" {
		return "@" + path
	}
	return "@" + path + " " + loc
}

func FormatYankAllContext(path, loc string, body []string) string {
	return fmt.Sprintf("%s\n\n%s", FormatYankLocation(path, loc), strings.Join(body, "\n"))
}
