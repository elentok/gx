package claudeui

import "testing"

func TestParseAddDirDialog(t *testing.T) {
	rows := []string{
		"Adding a new directory:", "",
		"/Users/x/Library/pnpm", "",
		"This session hasn't worked here before.", "",
		"  No, exit", "❯ Yes, add it",
	}
	d, ok := ParseAddDirDialog(rows)
	if !ok || d.Dir != "/Users/x/Library/pnpm" || d.ID == "" {
		t.Fatalf("got %+v, %v", d, ok)
	}
	again, _ := ParseAddDirDialog(append([]string{}, rows[:6]...))
	if _, ok := ParseAddDirDialog(rows[:1]); ok {
		t.Error("title alone must not parse")
	}
	_ = again
	other, _ := ParseAddDirDialog([]string{"Adding a new directory:", "/other", "❯ Yes, add it"})
	if other.ID == d.ID {
		t.Error("different directories share an id")
	}
}
