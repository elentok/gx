package worktrees

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// compressSegment shortens a single dash-delimited word: segments of 1-2 runes
// are kept as-is; longer segments keep their first and last rune and strip
// interior vowels.
func compressSegment(s string) string {
	runes := []rune(s)
	if len(runes) <= 2 {
		return s
	}
	var b strings.Builder
	b.WriteRune(runes[0])
	for _, r := range runes[1 : len(runes)-1] {
		switch unicode.ToLower(r) {
		case 'a', 'e', 'i', 'o', 'u':
		default:
			b.WriteRune(r)
		}
	}
	b.WriteRune(runes[len(runes)-1])
	return b.String()
}

func shortenName(name string, aliases map[string]string) string {
	if alias, ok := aliases[name]; ok {
		name = alias
	}
	parts := strings.Split(name, "-")
	for i, p := range parts {
		parts[i] = compressSegment(p)
	}
	return strings.TrimPrefix(strings.Join(parts, "-"), ".")
}

func sessionNameFor(repoName, wtName string, aliases map[string]string) string {
	name := shortenName(repoName, aliases) + "-" + shortenName(wtName, aliases)
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

func cmdKittySplit(wtPath string, horizontal bool) tea.Cmd {
	return func() tea.Msg {
		location := "vsplit"
		if horizontal {
			location = "hsplit"
		}
		err := exec.Command("kitty", "@", "launch", "--type=window",
			"--location="+location, "--no-response", "--cwd="+wtPath).Run()
		return terminalResultMsg{err: err}
	}
}

func cmdKittyNewTab(wtPath string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("kitty", "@", "launch", "--type=tab",
			"--no-response", "--cwd="+wtPath).Run()
		return terminalResultMsg{err: err}
	}
}
