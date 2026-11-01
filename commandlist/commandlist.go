// Package commandlist holds the slash-command entries the web UI offers for a
// session: each harness's built-in lists and the helper that merges them with
// a probe reply.
package commandlist

import "strings"

// Source says where a command comes from.
type Source string

const (
	SourceBuiltIn Source = "built-in"
	SourceSkill   Source = "skill"
	SourceMCP     Source = "mcp"
)

// Entry is one row of the command list. Name has no leading "/".
type Entry struct {
	Name        string
	ArgHint     string
	Description string
	Source      Source
	Aliases     []string
}

// List is a harness's built-in commands and the release they were taken from.
type List struct {
	Version string
	Entries []Entry
}

const (
	harnessClaude = "claude"
	harnessCodex  = "codex"
)

// BuiltinsFor returns the built-ins of the named harness for a server running
// on goos (a runtime.GOOS value). An unknown harness has none.
func BuiltinsFor(harnessName, goos string) List {
	switch harnessName {
	case harnessClaude:
		return List{Version: claudeVersion, Entries: claudeBuiltins}
	case harnessCodex:
		entries := make([]Entry, 0, len(codexBuiltins))
		for _, e := range codexBuiltins {
			if e.Name == "app" && goos != "darwin" && goos != "windows" {
				continue
			}
			entries = append(entries, e)
		}
		return List{Version: codexVersion, Entries: entries}
	}
	return List{}
}

var htmlUnescaper = strings.NewReplacer("&lt;", "<", "&gt;", ">")

// Merge combines the probe reply's entries with the built-ins. A built-in
// whose name or alias matches a reply entry's name or alias is dropped, so the
// reply wins. Descriptions are unescaped (the probes report "&lt;x&gt;").
// Reply entries come first; the web orders the rows.
func Merge(reply, builtins []Entry) []Entry {
	seen := map[string]bool{}
	out := make([]Entry, 0, len(reply)+len(builtins))
	for _, group := range [][]Entry{reply, builtins} {
		// Within a group every entry is kept; names only clash across groups.
		var added []string
		for _, e := range group {
			if clashes(seen, e) {
				continue
			}
			e.Description = htmlUnescaper.Replace(e.Description)
			out = append(out, e)
			added = append(added, e.Name)
			added = append(added, e.Aliases...)
		}
		for _, n := range added {
			seen[n] = true
		}
	}
	return out
}

func clashes(seen map[string]bool, e Entry) bool {
	if seen[e.Name] {
		return true
	}
	for _, a := range e.Aliases {
		if seen[a] {
			return true
		}
	}
	return false
}
