package herdr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reply is what the stub prints for one `herdr <a> <b>` command, and the exit
// code it ends with.
type reply struct {
	out  string
	exit int
}

// stubHerdr writes an executable `herdr` shell script into a temp dir and
// returns a CLI whose PATH is only that dir, plus the file the script appends
// each call's argv to (one line per call). replies are keyed by "$1 $2"; any
// other command exits 2.
func stubHerdr(t *testing.T, replies map[string]reply) (CLI, string) {
	t.Helper()
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	var sb strings.Builder
	fmt.Fprintf(&sb, "#!/bin/sh\necho \"$*\" >> %q\ncase \"$1 $2\" in\n", argvLog)
	// The PATH holds only the stub, so the script may use shell builtins only:
	// replies are single-quoted printf arguments, not files read with cat.
	for key, r := range replies {
		fmt.Fprintf(&sb, "%q) printf '%%s' '%s'; exit %d;;\n", key, strings.ReplaceAll(r.out, "'", `'\''`), r.exit)
	}
	sb.WriteString("*) echo unexpected command >&2; exit 2;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(sb.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return CLI{Env: []string{"PATH=" + dir}}, argvLog
}

// calls returns the argv lines the stub recorded.
func calls(t *testing.T, argvLog string) []string {
	t.Helper()
	b, err := os.ReadFile(argvLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
