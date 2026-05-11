package yankfmt

import (
	"fmt"
	"strings"
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
