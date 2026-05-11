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

func FocusedLocationAndBody(section DiffData, navMode NavMode) (string, []string, FocusedYankError) {
	hunkIdx := ActiveHunkIndexForYank(section, navMode)
	if hunkIdx < 0 || hunkIdx >= len(section.Parsed.Hunks) {
		return "", nil, FocusedYankErrNoHunk
	}
	body := FocusedYankBody(section, navMode)
	if len(body) == 0 {
		return "", nil, FocusedYankErrNoLines
	}
	loc := FocusedLocation(section, navMode)
	return loc, body, ""
}

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
