package commandlist_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/commandlist"
)

func names(l []commandlist.Entry) []string {
	var out []string
	for _, e := range l {
		out = append(out, e.Name)
	}
	return out
}

func find(l []commandlist.Entry, name string) (commandlist.Entry, bool) {
	for _, e := range l {
		if e.Name == name {
			return e, true
		}
	}
	return commandlist.Entry{}, false
}

func assertNoDuplicates(t *testing.T, l []commandlist.Entry) {
	t.Helper()
	seen := map[string]bool{}
	for _, e := range l {
		for _, n := range append([]string{e.Name}, e.Aliases...) {
			if seen[n] {
				t.Errorf("duplicate name or alias %q", n)
			}
			seen[n] = true
		}
	}
}

func TestBuiltinsFor_Claude(t *testing.T) {
	l := commandlist.BuiltinsFor("claude", "linux")
	if len(l.Entries) == 0 {
		t.Fatal("claude list is empty")
	}
	assertNoDuplicates(t, l.Entries)
	for _, want := range []string{"model", "exit", "help", "resume", "permissions"} {
		if !slices.Contains(names(l.Entries), want) {
			t.Errorf("claude list lacks %q", want)
		}
	}
}

func TestBuiltinsFor_Codex(t *testing.T) {
	l := commandlist.BuiltinsFor("codex", "linux")
	assertNoDuplicates(t, l.Entries)
	for _, hidden := range []string{"rollout", "test-approval", "debug-m-drop", "debug-m-update"} {
		if slices.Contains(names(l.Entries), hidden) {
			t.Errorf("codex list has hidden command %q", hidden)
		}
	}
	pwd, ok := find(l.Entries, "pwd")
	if !ok || !slices.Contains(pwd.Aliases, "cwd") {
		t.Errorf("pwd = %+v, want alias cwd", pwd)
	}
}

func TestBuiltinsFor_CodexAppOnlyOnDesktopOS(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "windows": true, "linux": false} {
		_, got := find(commandlist.BuiltinsFor("codex", goos).Entries, "app")
		if got != want {
			t.Errorf("app on %s = %v, want %v", goos, got, want)
		}
	}
}

func TestBuiltinsFor_Versions(t *testing.T) {
	for _, h := range []string{"claude", "codex"} {
		if commandlist.BuiltinsFor(h, "linux").Version == "" {
			t.Errorf("%s list has no version", h)
		}
	}
}

func TestMerge_ReplyWinsOnNameAndAlias(t *testing.T) {
	reply := []commandlist.Entry{
		{Name: "model", Description: "from reply"},
		{Name: "quit"},
	}
	builtins := []commandlist.Entry{
		{Name: "model", Description: "built-in"},
		{Name: "exit", Aliases: []string{"quit"}},
		{Name: "help"},
	}
	got := commandlist.Merge(reply, builtins)
	if want := []string{"model", "quit", "help"}; !slices.Equal(names(got), want) {
		t.Fatalf("names = %v, want %v", names(got), want)
	}
	if got[0].Description != "from reply" {
		t.Errorf("model description = %q, want the reply's", got[0].Description)
	}
}

func TestMerge_UnescapesDescriptions(t *testing.T) {
	got := commandlist.Merge([]commandlist.Entry{
		{Name: "spec", Description: "docs/specs/&lt;epic-slug&gt;.md"},
	}, []commandlist.Entry{{Name: "help", Description: "use &lt;x&gt;"}})
	if got[0].Description != "docs/specs/<epic-slug>.md" {
		t.Errorf("reply description = %q", got[0].Description)
	}
	if got[1].Description != "use <x>" {
		t.Errorf("built-in description = %q", got[1].Description)
	}
}
