package sched

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
)

// noteFiles are the two files of one ticket's launch note, in the ticket store
// beside the epic and outside every worktree: the note text, and the Claude
// prompt file that sends the agent to it.
type noteFiles struct{ note, prompt string }

func launchNoteFiles(storeDir string, a tickets.Address) noteFiles {
	dir := filepath.Join(storeDir, a.Epic, "launch")
	return noteFiles{note: filepath.Join(dir, a.ID+"-note.md"), prompt: filepath.Join(dir, a.ID+"-prompt.md")}
}

// remove deletes both files; a missing one is fine.
func (f noteFiles) remove() {
	_ = os.Remove(f.note)
	_ = os.Remove(f.prompt)
}

// applyLaunchNote stores note for the launch of a. A non-empty note is
// rewritten to its file. Claude gets a per-launch prompt file in place of the
// shared one, returned in the args. Codex has no prompt-file flag, so it gets
// the note file path back, for the runner to name in its developer
// instructions. An empty note removes any stale files and changes nothing.
func (l *Launcher) applyLaunchNote(a tickets.Address, note string, codex bool, args []string) (newArgs []string, noteFile string, err error) {
	dir, err := l.Projects.Dir(a.Project)
	if err != nil {
		return nil, "", err
	}
	files := launchNoteFiles(dir, a)
	if note == "" {
		files.remove()
		return args, "", nil
	}
	if err := os.MkdirAll(filepath.Dir(files.note), 0o755); err != nil {
		return nil, "", fmt.Errorf("launch note dir: %w", err)
	}
	if err := os.WriteFile(files.note, []byte(note+"\n"), 0o644); err != nil {
		return nil, "", fmt.Errorf("write launch note: %w", err)
	}
	if codex {
		return args, files.note, nil
	}
	if err := os.WriteFile(files.prompt, []byte(harness.GXLoopPromptWithNote(files.note)), 0o644); err != nil {
		return nil, "", fmt.Errorf("write launch prompt: %w", err)
	}
	return harness.WithPromptFile(args, files.prompt), "", nil
}

// removeLaunchNote drops a closed ticket's note files.
func (l *Launcher) removeLaunchNote(a tickets.Address) {
	if dir, err := l.Projects.Dir(a.Project); err == nil {
		launchNoteFiles(dir, a).remove()
	}
}
