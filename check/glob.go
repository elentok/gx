package check

import (
	"regexp"
	"strings"
)

// matchGlob reports whether the repo-relative slash path matches pattern.
// "*" and "?" stay inside one path segment; "**" crosses segments, and
// "**/" also matches no directory at all.
func matchGlob(pattern, path string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if strings.HasPrefix(pattern[i:], "**/") {
				b.WriteString("(.*/)?")
				i += 2
			} else if strings.HasPrefix(pattern[i:], "**") {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(path)
}

// appliesTo reports whether the entry runs for the changed files: an entry
// without globs always does.
func (a AlwaysEntry) appliesTo(files []string) bool {
	if len(a.Files) == 0 {
		return true
	}
	for _, f := range files {
		for _, g := range a.Files {
			if matchGlob(g, f) {
				return true
			}
		}
	}
	return false
}
