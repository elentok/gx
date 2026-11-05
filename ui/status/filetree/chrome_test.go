package filetree

import (
	"testing"

	"github.com/elentok/gx/git/stage"
)

func TestStatusFileIconDeletedAndFallback(t *testing.T) {
	t.Parallel()
	deleted := stage.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'}

	nerd := paneIconsFor(true)
	if got := fileIcon(deleted, false, nerd); got != "\U0000F4D6" {
		t.Fatalf("expected deleted nerd icon, got %q", got)
	}

	plain := paneIconsFor(false)
	if got := fileIcon(deleted, false, plain); got != "D" {
		t.Fatalf("expected deleted fallback icon, got %q", got)
	}
}

func TestStatusEntryColorDeletedFileIsDim(t *testing.T) {
	t.Parallel()
	entry := Entry{
		Kind: EntryFile,
		File: stage.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'},
	}
	if got := entryColor(entry); got != "#a6adc8" {
		t.Fatalf("expected dim deleted color, got %q", got)
	}
}

func TestStatusFileIconRenamedAndFallback(t *testing.T) {
	t.Parallel()
	renamed := stage.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'}

	nerd := paneIconsFor(true)
	if got := fileIcon(renamed, false, nerd); got != "\U000F0054" {
		t.Fatalf("expected renamed nerd icon, got %q", got)
	}

	plain := paneIconsFor(false)
	if got := fileIcon(renamed, false, plain); got != "R" {
		t.Fatalf("expected renamed fallback icon, got %q", got)
	}
}

func TestStatusEntryColorRenamedFileIsBlue(t *testing.T) {
	t.Parallel()
	entry := Entry{
		Kind: EntryFile,
		File: stage.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'},
	}
	if got := entryColor(entry); got != "#89b4fa" {
		t.Fatalf("expected renamed color, got %q", got)
	}
}

func TestStatusEntryColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{name: "unstaged", entry: Entry{HasUnstaged: true}, want: "#cdd6f4"},
		{name: "partial", entry: Entry{HasStaged: true, HasUnstaged: true}, want: "#fab387"},
		{name: "staged", entry: Entry{HasStaged: true}, want: "#a6e3a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entryColor(tt.entry); got != tt.want {
				t.Fatalf("statusEntryColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEntryMeta_NerdFont(t *testing.T) {
	t.Parallel()
	icons := paneIconsFor(true)

	if got := entryMeta(Entry{HasStaged: true, HasUnstaged: true}, true, icons); got != "\U0000F00C" {
		t.Fatalf("partial nerd icon = %q", got)
	}
	if got := entryMeta(Entry{HasStaged: true}, true, icons); got != "\U0000EE29" {
		t.Fatalf("staged nerd icon = %q", got)
	}
}

func TestStatusFileIcon(t *testing.T) {
	t.Parallel()
	icons := paneIconsFor(true)

	if got := fileIcon(stage.StageFileStatus{IndexStatus: '?', WorktreeCode: '?'}, false, icons); got != "\U0000F4D0" {
		t.Fatalf("untracked icon = %q, want new file icon", got)
	}
	if got := fileIcon(stage.StageFileStatus{IndexStatus: 'A', WorktreeCode: ' '}, false, icons); got != "\U0000F4D0" {
		t.Fatalf("added icon = %q, want new file icon", got)
	}
	if got := fileIcon(stage.StageFileStatus{IndexStatus: ' ', WorktreeCode: 'M'}, false, icons); got != "\U0000F4D2" {
		t.Fatalf("modified icon = %q, want modified file icon", got)
	}
}

func TestStatusFileIconSymlink(t *testing.T) {
	t.Parallel()
	nerd := paneIconsFor(true)
	plain := paneIconsFor(false)

	modified := stage.StageFileStatus{Path: "link", IndexStatus: ' ', WorktreeCode: 'M'}
	if got := fileIcon(modified, true, nerd); got != "\U000F0337" {
		t.Fatalf("symlink nerd icon = %q, want symlink icon", got)
	}
	if got := fileIcon(modified, true, plain); got != "L" {
		t.Fatalf("symlink plain icon = %q, want L", got)
	}

	// Deleted symlink keeps the delete icon (deletion takes precedence).
	deleted := stage.StageFileStatus{Path: "link", IndexStatus: 'D'}
	if got := fileIcon(deleted, true, nerd); got == "\U000F0337" {
		t.Fatalf("deleted symlink should show delete icon, not symlink icon")
	}
}
