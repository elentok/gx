package notifyhistory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/notifylog"
)

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (m Model) export() (Model, tea.Cmd) {
	path, err := writeExport(m.visibleEntries(), m.repoName, m.worktreeName)
	if err != nil {
		return m, notify.Error("export notifications: " + err.Error())
	}
	return m, notify.Success("exported to " + path)
}

func writeExport(entries []notifylog.Entry, repoName, worktreeName string) (string, error) {
	base, err := config.UserCacheDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(base, "gx")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s-%s.md", ts, sanitizeFilename(repoName, "repo"), sanitizeFilename(worktreeName, "worktree"))
	path := filepath.Join(cacheDir, name)
	if err := os.WriteFile(path, []byte(formatMarkdown(entries)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func formatMarkdown(entries []notifylog.Entry) string {
	lines := make([]string, 0, len(entries)+2)
	lines = append(lines, "# Notification history", "")
	for _, e := range entries {
		lines = append(lines, "- "+entryText(e))
	}
	return strings.Join(lines, "\n") + "\n"
}

func sanitizeFilename(name, fallback string) string {
	clean := invalidFilenameChars.ReplaceAllString(strings.TrimSpace(name), "-")
	clean = strings.Trim(clean, "-.")
	if clean == "" {
		return fallback
	}
	return clean
}
