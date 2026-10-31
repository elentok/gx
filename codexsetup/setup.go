package codexsetup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options configures Run.
type Options struct {
	CodexHome   string
	GoplsServer string // "" picks gopls-mcp, else gopls
	Yes         bool   // skip the question
	Interactive bool   // stdin is a terminal
	In          io.Reader
	Out         io.Writer
}

type change struct {
	path     string
	old, new string
	existed  bool
}

// Run previews the changes, asks, then writes them.
func Run(o Options) error {
	changes, notes, err := plan(o.CodexHome, o.GoplsServer)
	if err != nil {
		return err
	}
	for _, n := range notes {
		fmt.Fprintln(o.Out, n)
	}
	if len(changes) == 0 {
		if len(notes) == 0 {
			fmt.Fprintln(o.Out, "already set up")
		} else {
			fmt.Fprintln(o.Out, "nothing to write")
		}
		return nil
	}
	for _, c := range changes {
		preview(o.Out, c)
	}
	if !o.Yes {
		if !o.Interactive {
			return errors.New("refusing to write without a terminal; pass --yes to confirm")
		}
		fmt.Fprint(o.Out, "Write these changes? [y/N] ")
		line, _ := bufio.NewReader(o.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(o.Out, "Aborted; nothing written.")
			return nil
		}
	}
	for _, c := range changes {
		if err := writeAtomic(c.path, []byte(c.new)); err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "Wrote %s\n", c.path)
	}
	fmt.Fprintln(o.Out, "Restart Codex for the changes to take effect.")
	return nil
}

// plan computes the file changes. Notes explain what was skipped.
func plan(codexHome, gopls string) (changes []change, notes []string, err error) {
	rulesPath := RulesPath(codexHome)
	old, rerr := os.ReadFile(rulesPath)
	switch {
	case rerr == nil && !bytes.Contains(old, []byte(Marker)) && !bytes.Contains(old, []byte(OldMarker)):
		notes = append(notes, fmt.Sprintf("%s exists and is not managed by gx; left untouched", rulesPath))
	case rerr == nil || errors.Is(rerr, os.ErrNotExist):
		if c := (change{path: rulesPath, old: string(old), new: RulesContent(), existed: rerr == nil}); c.old != c.new {
			changes = append(changes, c)
		}
	default:
		return nil, nil, rerr
	}

	cfgPath := filepath.Join(codexHome, "config.toml")
	cfg, cerr := os.ReadFile(cfgPath)
	if errors.Is(cerr, os.ErrNotExist) {
		return changes, append(notes, fmt.Sprintf("%s does not exist; skipped gopls approvals", cfgPath)), nil
	}
	if cerr != nil {
		return nil, nil, cerr
	}
	server := resolveServer(strings.Split(string(cfg), "\n"), gopls)
	if server == "" {
		return changes, append(notes, missingServerMessage(gopls, cfgPath)), nil
	}
	if updated := approveGopls(string(cfg), server); updated != string(cfg) {
		changes = append(changes, change{path: cfgPath, old: string(cfg), new: updated, existed: true})
	}
	return changes, notes, nil
}

func preview(w io.Writer, c change) {
	if !c.existed {
		fmt.Fprintf(w, "\n%s (new file):\n%s", c.path, c.new)
		if !strings.HasSuffix(c.new, "\n") {
			fmt.Fprintln(w)
		}
		return
	}
	fmt.Fprintf(w, "\n%s:\n%s", c.path, lineDiff(c.old, c.new))
}

func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// lineDiff prints removed lines as "-", added lines as "+" and two lines of
// context around each change.
func lineDiff(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []string // each is a marker byte followed by the line
	for i, j := 0, 0; i < len(x) || j < len(y); {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			ops = append(ops, " "+x[i])
			i++
			j++
		case j < len(y) && (i == len(x) || lcs[i][j+1] >= lcs[i+1][j]):
			ops = append(ops, "+"+y[j])
			j++
		default:
			ops = append(ops, "-"+x[i])
			i++
		}
	}
	const ctx = 2
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op[0] == ' ' {
			continue
		}
		for k := max(0, i-ctx); k <= min(len(ops)-1, i+ctx); k++ {
			keep[k] = true
		}
	}
	var sb strings.Builder
	skipped := false
	for i, op := range ops {
		if !keep[i] {
			skipped = true
			continue
		}
		if skipped {
			sb.WriteString("...\n")
			skipped = false
		}
		sb.WriteString(op)
		sb.WriteByte('\n')
	}
	return sb.String()
}
