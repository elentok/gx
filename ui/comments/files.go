package comments

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func Write(path, loc string, body []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	commentsDir := filepath.Join(home, ".local", "share", "gx", "comments")
	if err := os.MkdirAll(commentsDir, 0o755); err != nil {
		return "", err
	}
	base := sanitizeFilename(filepath.Base(path))
	if base == "" {
		base = "file"
	}
	ts := time.Now().Format("20060102-150405")
	filePath := filepath.Join(commentsDir, fmt.Sprintf("%s-%s.md", ts, base))
	for i := 2; ; i++ {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			break
		}
		filePath = filepath.Join(commentsDir, fmt.Sprintf("%s-%s-%d.md", ts, base, i))
	}
	content := formatMarkdown(path, loc, body)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return filePath, nil
}

func formatMarkdown(path, loc string, body []string) string {
	header := "@" + path
	if loc != "" {
		header += " " + loc
	}
	lines := []string{header, "", "```diff"}
	lines = append(lines, body...)
	lines = append(lines, "```", "")
	return strings.Join(lines, "\n")
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	clean := invalidFilenameChars.ReplaceAllString(name, "-")
	clean = strings.Trim(clean, "-.")
	return clean
}
