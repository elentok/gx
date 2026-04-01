package stage

import (
	"fmt"
	"path/filepath"
	"strings"
)

func editorLaunchArgs(editorBin string, editorArgs []string, target string, line int) []string {
	if line <= 0 {
		return append(editorArgs, target)
	}
	base := strings.ToLower(filepath.Base(editorBin))
	base = strings.TrimSuffix(base, ".exe")
	gotoPath := fmt.Sprintf("%s:%d", target, line)

	switch base {
	case "code", "code-insiders", "codium", "cursor", "windsurf":
		args := append([]string{}, editorArgs...)
		args = append(args, "--goto", gotoPath)
		return args
	case "vim", "nvim", "vi", "nano", "hx", "helix", "kak":
		args := append([]string{}, editorArgs...)
		args = append(args, fmt.Sprintf("+%d", line), target)
		return args
	case "subl", "sublime_text", "mate", "zed":
		args := append([]string{}, editorArgs...)
		args = append(args, gotoPath)
		return args
	default:
		return append(editorArgs, target)
	}
}
