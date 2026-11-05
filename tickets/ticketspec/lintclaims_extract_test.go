package ticketspec

import (
	"reflect"
	"testing"
)

func TestExtractClaims_Spans(t *testing.T) {
	top := []string{"tickets", "git", "docs", "server"}
	tests := []struct {
		name string
		body string
		want []Claim
	}{
		{"go selector", "`schema.StatusDraft` `tickets.Work`", nil},
		{"glob", "`webui/e2e/*.spec.ts`", nil},
		{"shell command", "`git rev-parse --short HEAD`", nil},
		{"ext without slash", "`SKILL.md` `report_metrics.go`", nil},
		{"number", "`0.83`", nil},
		{"project:epic", "`gx:ticket-authoring-tools-impl`", nil},
		{"url and tilde and dollar", "`https://x.io/a.md` `~/a/b.md` `$HOME/a.go`", nil},
		{"dir without slash", "`tickets` `git`", nil},
		{"unticked path", "see tickets/nosuch.go for more", nil},
		{"missing go path", "`tickets/nosuch.go`", []Claim{{Kind: ClaimPath, Text: "tickets/nosuch.go", File: "tickets/nosuch.go"}}},
		{"missing md path", "`docs/nowhere.md`", []Claim{{Kind: ClaimPath, Text: "docs/nowhere.md", File: "docs/nowhere.md"}}},
		{"top dir path", "`server/handlers`", []Claim{{Kind: ClaimPath, Text: "server/handlers", File: "server/handlers"}}},
		{"unknown dir, no ext", "`foo/bar`", nil},
		{"command", "`gx tickets nosuch`", []Claim{{Kind: ClaimCommand, Text: "gx tickets nosuch", Words: []string{"tickets", "nosuch"}}}},
		{"command flag", "`gx tickets add <epic> --nosuch=3`", []Claim{{Kind: ClaimCommand, Text: "gx tickets add <epic> --nosuch=3", Words: []string{"tickets", "add"}, Flags: []string{"--nosuch"}}}},
		{"run single", "`-run 'TestNoSuch'`", []Claim{{Kind: ClaimTestPattern, Text: "-run 'TestNoSuch'", Pattern: "TestNoSuch"}}},
		{"run double", `` + "`-run \"TestA|TestB\"`", []Claim{{Kind: ClaimTestPattern, Text: `-run "TestA|TestB"`, Pattern: "TestA|TestB"}}},
		{"file:func", "`tickets/x.go:Foo`", []Claim{{Kind: ClaimFileFunc, Text: "tickets/x.go:Foo", File: "tickets/x.go", Func: "Foo"}}},
		{"file:N", "`server/x.go:42`", []Claim{{Kind: ClaimLineRef, Text: "server/x.go:42", File: "server/x.go"}}},
		{"file:N-M", "`server/x.go:42-50`", []Claim{{Kind: ClaimLineRef, Text: "server/x.go:42-50", File: "server/x.go"}}},
		{"flow", "Covers flow 999.", []Claim{{Kind: ClaimFlow, Text: "flow 999", Flow: 999}}},
		{"new flow", "Adds new flow 5.", []Claim{{Kind: ClaimFlow, Text: "new flow 5", Flow: 5, New: true}}},
		{"flow inside a span is not prose", "`flow 7`", nil},
		{"prose line", "see line 42", []Claim{{Kind: ClaimLineRef, Text: "line 42"}}},
		{"fenced block skipped", "```\n`tickets/in-fence.go` flow 3\n```", nil},
		{"go fence skipped", "```go\n`tickets/in-fence.go`\n```", nil},
		{"text fence read", "```text\n`tickets/in-fence.go`\n```", []Claim{{Kind: ClaimPath, Text: "tickets/in-fence.go", File: "tickets/in-fence.go"}}},
		{"run with dollar anchor", "`-run 'TestX$'`", []Claim{{Kind: ClaimTestPattern, Text: "-run 'TestX$'", Pattern: "TestX$"}}},
		{"run with star", "`-run 'Test.*'`", []Claim{{Kind: ClaimTestPattern, Text: "-run 'Test.*'", Pattern: "Test.*"}}},
		{"command stops at $ argument", "`gx tickets show $E`", []Claim{{Kind: ClaimCommand, Text: "gx tickets show $E", Words: []string{"tickets", "show"}}}},
		{"path with star or dollar skipped", "`tickets/*.go` `tickets/$X.go`", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractClaims(tt.body, top); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExtractClaims_ReadsEverySectionAndRecordsIt(t *testing.T) {
	body := "intro `docs/a.md`\n\n## What to build\n\n`docs/b.md`\n\n## Where to look\n\n`docs/c.md`\n\n## Acceptance criteria\n\n`docs/d.md`\n"
	got := ExtractClaims(body, []string{"docs"})
	var pairs [][2]string
	for _, c := range got {
		pairs = append(pairs, [2]string{c.File, c.Section})
	}
	want := [][2]string{{"docs/a.md", ""}, {"docs/b.md", "What to build"}, {"docs/c.md", "Where to look"}, {"docs/d.md", "Acceptance criteria"}}
	if !reflect.DeepEqual(pairs, want) {
		t.Errorf("got %v, want %v", pairs, want)
	}
}
