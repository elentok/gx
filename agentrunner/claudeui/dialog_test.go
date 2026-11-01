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

func TestReadDialog_Goldens(t *testing.T) {
	tests := []struct {
		file     string
		kind     DialogKind
		header   string
		options  int
		plainYes bool
	}{
		{"permission-w160.screen.txt", DialogPermission, "Bash command", 4, true},
		{"dialog-edit.screen.txt", DialogPermission, "Edit file", 3, true},
		{"dialog-create.screen.txt", DialogPermission, "Create file", 3, true},
		{"dialog-read.screen.txt", DialogPermission, "Read file", 3, true},
		{"dialog-unknown.screen.txt", DialogPermission, "Frobnicate widget", 2, true},
		{"dialog-askuserquestion.screen.txt", DialogUnreadable, "", 0, false},
		{"plan-approval.screen.txt", DialogPlan, "", 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			d := ReadDialog(dialogRows(t, tt.file))
			if d.Kind != tt.kind || d.Header != tt.header || len(d.Options) != tt.options || d.PlainYes != tt.plainYes {
				t.Fatalf("got kind=%q header=%q options=%d plainYes=%v, want %q %q %d %v",
					d.Kind, d.Header, len(d.Options), d.PlainYes, tt.kind, tt.header, tt.options, tt.plainYes)
			}
			if (d.ID != "") != (tt.kind != DialogUnreadable) {
				t.Fatalf("ID %q does not match kind %q", d.ID, d.Kind)
			}
			if tt.kind != DialogUnreadable && d.Cursor != 0 {
				t.Fatalf("cursor = %d, want 0", d.Cursor)
			}
		})
	}
}

func TestReadDialog_CursorOnOptionTwo(t *testing.T) {
	rows := dialogRows(t, "permission-w160.screen.txt")
	for i, r := range rows {
		switch {
		case strings.Contains(r, "❯ 1."):
			rows[i] = strings.Replace(r, "❯ 1.", "  1.", 1)
		case strings.Contains(r, "2."):
			rows[i] = strings.Replace(r, "  2.", "❯ 2.", 1)
		}
	}
	if d := ReadDialog(rows); d.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", d.Cursor)
	}
}

func TestReadDialog_IDMatchesParseDialog(t *testing.T) {
	rows := dialogRows(t, "permission-w160.screen.txt")
	if got, want := ReadDialog(rows).ID, ParseDialog(rows).ID; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
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

func TestParseDialog_WordWrappedBodyKeepsIDAndText(t *testing.T) {
	tail := []string{"Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"}
	wide := ParseDialog(append([]string{"────", "Bash command", "  rg --files --hidden ./internal/some/deep/path ./other"}, tail...))
	narrow := ParseDialog(append([]string{"────", "Bash command", "  rg --files --hidden", "  ./internal/some/deep/path ./other"}, tail...))
	if !wide.Approvable || narrow.Body != wide.Body || narrow.ID != wide.ID {
		t.Fatalf("narrow = %+v, wide = %+v; want the same body and ID", narrow, wide)
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
