// Package modelrules holds the pure rules for switching a session's model:
// parsing ids, finding the latest model of a family and carrying effort over.
// It reads no process, file or clock.
package modelrules

import (
	"regexp"
	"slices"
	"strconv"
)

// Version is a parsed model version. Compare with Compare, never as text.
type Version struct{ Major, Minor int }

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return cmpInt(v.Major, o.Major)
	}
	return cmpInt(v.Minor, o.Minor)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

var (
	dateSuffix  = regexp.MustCompile(`-\d{8}$`)
	codexShape  = regexp.MustCompile(`^gpt-(\d+)(?:\.(\d+))?-([a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*)$`)
	claudeShape = regexp.MustCompile(`^claude-([a-z][a-z0-9]*)-(\d+)(?:-(\d+))?$`)
)

// ParseID returns the family and version of a model id. ok is false for any
// id that is not one of the two strict shapes (including aliases).
func ParseID(id string) (family string, v Version, ok bool) {
	id = dateSuffix.ReplaceAllString(id, "")
	m := codexShape.FindStringSubmatch(id)
	if m != nil {
		return m[3], mkVersion(m[1], m[2]), true
	}
	if m = claudeShape.FindStringSubmatch(id); m != nil {
		return m[1], mkVersion(m[2], m[3]), true
	}
	return "", Version{}, false
}

func mkVersion(major, minor string) Version {
	maj, _ := strconv.Atoi(major)
	mn, _ := strconv.Atoi(minor)
	return Version{Major: maj, Minor: mn}
}

// Entry is one discovered model.
type Entry struct {
	Value     string // what a launch takes (an alias or a resolved id)
	ID        string // the resolved id
	Hidden    bool
	Successor string // the successor the harness names, or ""
}

// Latest returns the latest model of the family of model, as a launch value.
// ok is false ("none") for every doubt: no family, no list, or model already
// the latest.
func Latest(model string, list []Entry) (value string, ok bool) {
	resolved := model
	var cur *Entry
	for i := range list {
		if list[i].Value == model {
			cur = &list[i]
			break
		}
	}
	if cur == nil {
		for i := range list {
			if list[i].ID == model {
				cur = &list[i]
				break
			}
		}
	}
	if cur != nil && cur.ID != "" {
		resolved = cur.ID
	}
	family, ver, parsed := ParseID(resolved)
	if !parsed || len(list) == 0 {
		return "", false
	}

	if cur != nil && cur.Successor != "" {
		for _, e := range list {
			if !e.Hidden && (e.Value == cur.Successor || e.ID == cur.Successor) && e.ID != resolved {
				return e.Value, true
			}
		}
	}

	best := -1
	bestVer := ver
	for i, e := range list {
		if e.Hidden || e.Value != e.ID {
			continue
		}
		f, v, okp := ParseID(e.ID)
		if !okp || f != family {
			continue
		}
		if v.Compare(bestVer) > 0 {
			best, bestVer = i, v
		}
	}
	if best < 0 {
		return "", false
	}
	return list[best].Value, true
}

var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// Effort returns the effort to use on the new model. codex selects the Codex
// rule (the new model's default) over the Claude rule (nearest lower level).
func Effort(current string, supported []string, codex bool, codexDefault string) string {
	if len(supported) == 0 {
		return ""
	}
	if current == "" || slices.Contains(supported, current) {
		return current
	}
	if codex {
		return codexDefault
	}
	cur := slices.Index(claudeEfforts, current)
	lowest := ""
	lowestIdx := len(claudeEfforts)
	nearest := ""
	nearestIdx := -1
	for _, s := range supported {
		i := slices.Index(claudeEfforts, s)
		if i < 0 {
			continue
		}
		if i < lowestIdx {
			lowest, lowestIdx = s, i
		}
		if cur >= 0 && i < cur && i > nearestIdx {
			nearest, nearestIdx = s, i
		}
	}
	if nearest != "" {
		return nearest
	}
	return lowest
}
