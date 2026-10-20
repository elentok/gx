package claudeui

import (
	"os"
	"strings"
	"testing"
)

func dialogRows(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func TestParseDialog_Goldens(t *testing.T) {
	tests := []struct {
		file       string
		header     string
		options    int
		approvable bool
	}{
		{"permission-w160.screen.txt", "Bash command", 4, true},
		{"dialog-edit.screen.txt", "Edit file", 3, true},
		{"dialog-create.screen.txt", "Create file", 3, true},
		{"dialog-read.screen.txt", "Read file", 3, false},
		{"dialog-unknown.screen.txt", "Frobnicate widget", 2, false},
		{"dialog-askuserquestion.screen.txt", "", 0, false},
		{"plan-approval.screen.txt", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			d := ParseDialog(dialogRows(t, tt.file))
			if d.Header != tt.header || len(d.Options) != tt.options || d.Approvable != tt.approvable {
				t.Fatalf("got header=%q options=%d approvable=%v, want %q %d %v",
					d.Header, len(d.Options), d.Approvable, tt.header, tt.options, tt.approvable)
			}
			if (d.ID != "") != tt.approvable {
				t.Fatalf("ID %q set=%v, want set=%v", d.ID, d.ID != "", tt.approvable)
			}
		})
	}
}

func TestParseDialog_BashBody(t *testing.T) {
	d := ParseDialog(dialogRows(t, "permission-w160.screen.txt"))
	want := "Remove nonexistent file if present and echo marker rm -f gxwf-nonexistent.txt && echo gxwf-done"
	if d.Body != want {
		t.Fatalf("body = %q, want %q", d.Body, want)
	}
}

func TestParseDialog_IDIgnoresTipCwdAndFooter(t *testing.T) {
	rows := dialogRows(t, "permission-w160.screen.txt")
	base := ParseDialog(rows).ID

	var varied []string
	for _, r := range rows {
		switch {
		case strings.Contains(r, "Tip:"):
			r = " Tip: something else entirely"
		case strings.Contains(r, "Esc to cancel"):
			r = " Esc to cancel · Tab to amend · ctrl+e to explain"
		case strings.Contains(r, "Bash command"):
			varied = append(varied, r, " cwd: /somewhere/else")
			continue
		}
		varied = append(varied, r)
	}
	if got := ParseDialog(varied).ID; got != base {
		t.Fatalf("ID changed with tip/cwd/footer: %q vs %q", got, base)
	}
}

func TestParseDialog_IDChangesWithBody(t *testing.T) {
	rows := dialogRows(t, "permission-w160.screen.txt")
	base := ParseDialog(rows).ID

	var changed []string
	for _, r := range rows {
		changed = append(changed, strings.ReplaceAll(r, "echo gxwf-done", "echo other"))
	}
	if got := ParseDialog(changed).ID; got == base {
		t.Fatalf("ID did not change when the command changed: %q", got)
	}
}

func TestParseDialog_OptionOneMustBeExactlyYes(t *testing.T) {
	rows := dialogRows(t, "permission-w160.screen.txt")
	for i, r := range rows {
		rows[i] = strings.Replace(r, "1. Yes", "1. Yes, always", 1)
	}
	if d := ParseDialog(rows); d.Approvable {
		t.Fatal("option 1 other than \"Yes\" must not be approvable")
	}
}
