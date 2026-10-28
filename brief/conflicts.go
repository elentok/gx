package brief

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
)

// Conflicts prints the in-progress operation and each unmerged file with the
// line numbers of its conflict markers. It exits 1 while conflicts remain.
func Conflicts(dir string, w io.Writer) int {
	top, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return cantRun(w, "not a git repository")
	}
	files, err := gitOut(top, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return cantRun(w, err.Error())
	}
	op := inProgressOp(top)
	unmerged := strings.Fields(files)
	if len(unmerged) == 0 {
		fmt.Fprintf(w, "no conflicts (%s)\n", op)
		return ExitPass
	}
	fmt.Fprintf(w, "%d unmerged file(s) (%s)\n", len(unmerged), op)
	for _, f := range unmerged {
		fmt.Fprintln(w, f)
		for _, m := range markerLines(filepath.Join(top, f)) {
			fmt.Fprintf(w, "  %s\n", m)
		}
	}
	return ExitFail
}

func gitOut(dir string, args ...string) (string, error) {
	out, _, err := git.Run(dir, git.NonInteractiveEnv(), args...)
	return strings.TrimSpace(out), err
}

func inProgressOp(top string) string {
	for _, c := range []struct{ path, name string }{
		{"MERGE_HEAD", "merge"},
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
	} {
		p, err := gitOut(top, "rev-parse", "--git-path", c.path)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(top, p)
		}
		if _, err := os.Stat(p); err == nil {
			return c.name
		}
	}
	return "no merge in progress"
}

// markerLines returns "N:<marker>" for each conflict marker line in a file.
func markerLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		for _, m := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
			if strings.HasPrefix(line, m) {
				out = append(out, fmt.Sprintf("%d:%s", n, m))
			}
		}
	}
	if sc.Err() != nil {
		return nil
	}
	return out
}
