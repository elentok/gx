package worktrees

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func sessionNamePart(name string, aliases map[string]string) string {
	if alias, ok := aliases[name]; ok {
		name = alias
	}
	return strings.TrimPrefix(name, ".")
}

func sessionNameFor(repoName, wtName string, aliases map[string]string) string {
	name := sessionNamePart(repoName, aliases) + "-" + sessionNamePart(wtName, aliases)
	return strings.TrimPrefix(name, ".")
}

func kittySessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "kitty", "sessions"), nil
}

func kittySessionFile(name string) string {
	return name + ".kitty-session"
}

func cmdKittySession(name, wtPath string) tea.Cmd {
	return func() tea.Msg {
		dir, err := kittySessionDir()
		if err != nil {
			return terminalResultMsg{err: err}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return terminalResultMsg{err: err}
		}
		sessionFile := filepath.Join(dir, kittySessionFile(name))
		if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
			content := fmt.Sprintf("cd %s\nlaunch\n", wtPath)
			if err := os.WriteFile(sessionFile, []byte(content), 0o644); err != nil {
				return terminalResultMsg{err: err}
			}
		}
		err = exec.Command("kitten", "@", "action", "goto_session", sessionFile).Run()
		return terminalResultMsg{err: err}
	}
}
