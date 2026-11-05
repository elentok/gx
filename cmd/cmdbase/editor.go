package cmdbase

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RunEditorCommand runs editor (which may carry arguments, e.g. "code --wait")
// on path with env as the child's whole environment.
func RunEditorCommand(env []string, editor, path string, in io.Reader, out, errOut io.Writer) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("$EDITOR is empty")
	}
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Env = append([]string{}, env...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", editor, err)
	}
	return nil
}
