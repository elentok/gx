// Package herdr wraps the herdr CLI's workspace commands as plain functions
// returning (result, error). herdr is only a terminal multiplexer here: gx
// uses it to open a workspace and to check that it answers.
package herdr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CLI runs herdr under an explicit environment. The zero CLI is the process
// environment, which is what the package-level functions use.
type CLI struct {
	// Env is the child's environment as KEY=VALUE entries, like exec.Cmd.Env.
	// Its PATH finds the herdr binary. Nil means the process environment.
	Env []string
}

// fallbackDirs are where herdr is installed when the process's PATH lacks it,
// as under launchd (PATH=/usr/bin:/bin:/usr/sbin:/sbin).
var fallbackDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// lookPath finds herdr on c's PATH.
func (c CLI) lookPath() (string, error) {
	if c.Env == nil {
		return exec.LookPath("herdr")
	}
	for i := len(c.Env) - 1; i >= 0; i-- {
		path, ok := strings.CutPrefix(c.Env[i], "PATH=")
		if !ok {
			continue
		}
		for _, dir := range filepath.SplitList(path) {
			p := filepath.Join(dir, "herdr")
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return p, nil
			}
		}
		break
	}
	return "", exec.ErrNotFound
}

// binary resolves the herdr executable: PATH first, then HERDR_BIN_PATH (set
// inside a herdr pane), then the usual install dirs. It returns "herdr" when
// nothing is found, so exec reports the usual not-found error.
func (c CLI) binary() string {
	if p, err := c.lookPath(); err == nil {
		return p
	}
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, dir := range fallbackDirs {
		p := filepath.Join(dir, "herdr")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "herdr"
}

// run shells out to herdr with args and returns its combined output,
// wrapping a non-zero exit with the command line and output for context.
func (c CLI) run(args ...string) ([]byte, error) {
	cmd := exec.Command(c.binary(), args...)
	cmd.Env = c.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("$ herdr %s\n\n%w\n\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// runJSON shells out to herdr with args and unmarshals the `result` field of
// its JSON response into result (a pointer), unless result is nil.
func (c CLI) runJSON(args []string, result any) error {
	out, err := c.run(args...)
	if err != nil {
		return err
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("parsing herdr %s output: %w", strings.Join(args, " "), err)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("parsing herdr %s result: %w", strings.Join(args, " "), err)
	}
	return nil
}
