package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

func newProjectEditConfigCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-config [project]",
		Short: "edit a project's project.json in $EDITOR, validated before it is saved (project defaults to the current repo's)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runProjectEditConfig(d, name)
		},
	}
}

// runProjectEditConfig edits a temp copy of project.json so neither the server
// nor a concurrent writer ever sees a half-written or invalid file.
func runProjectEditConfig(d deps, name string) error {
	dir, err := projectDirFor(d, name)
	if err != nil {
		return err
	}
	livePath := filepath.Join(dir, config.ProjectFileName)
	orig, err := os.ReadFile(livePath)
	if os.IsNotExist(err) {
		return tickets.ErrNoProject
	}
	if err != nil {
		return err
	}
	editor := d.getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return fmt.Errorf("$EDITOR is not set")
	}
	tmp, err := os.CreateTemp("", "gx-project-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, err = tmp.Write(orig)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	keepTmp := false
	defer func() {
		if !keepTmp {
			os.Remove(tmpPath)
		}
	}()
	if err != nil {
		return err
	}

	tty := d.stdinIsTerminal != nil && d.stdinIsTerminal()
	answers := bufio.NewReader(d.stdin)
	for {
		if err := d.runEditor(editor, tmpPath, d.stdin, d.stdout, d.stderr); err != nil {
			return err
		}
		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			return err
		}
		if bytes.Equal(edited, orig) {
			fmt.Fprintln(d.stdout, "no changes")
			return nil
		}
		if err := validateProjectConfig(edited); err != nil {
			if !tty {
				keepTmp = true
				return fmt.Errorf("invalid project.json, not saved; your edits are in %s: %w", tmpPath, err)
			}
			fmt.Fprintf(d.stderr, "invalid project.json: %v\n", err)
			if !askEditAgain(answers, d.stdout) {
				fmt.Fprintln(d.stdout, "edits discarded; project.json unchanged")
				return nil
			}
			continue
		}
		live, err := os.ReadFile(livePath)
		if err != nil || !bytes.Equal(live, orig) {
			keepTmp = true
			return fmt.Errorf("project.json changed while you were editing; your edits are in %s", tmpPath)
		}
		if err := fsutil.WriteFileAtomic(livePath, edited); err != nil {
			return err
		}
		fmt.Fprintf(d.stdout, "saved %s\n", livePath)
		warnTrunkChange(d.stdout, dir, orig, edited)
		return nil
	}
}

// warnTrunkChange prints how many roots in flight a trunk change moves. Both
// files already passed validation, so their parse errors are ignored.
func warnTrunkChange(out io.Writer, dir string, orig, edited []byte) {
	before, _ := config.ParseProjectFile(orig)
	after, _ := config.ParseProjectFile(edited)
	repo := ""
	if before.Repo != nil {
		repo = *before.Repo
	}
	oldTrunk, newTrunk := effectiveTrunk(before, repo), effectiveTrunk(after, repo)
	if oldTrunk == newTrunk {
		return
	}
	epics, _ := tickets.Load(dir)
	fmt.Fprintf(out, "%d tickets in flight will land on %s\n", rootsInFlight(epics, repo, oldTrunk), newTrunk)
}

// effectiveTrunk mirrors tickets.Trunk without its existence check, so
// setting trunk to the detected default is not a change.
func effectiveTrunk(pf config.ProjectFile, repo string) string {
	if pf.Trunk != nil && *pf.Trunk != "" {
		return *pf.Trunk
	}
	if repo == "" {
		return ""
	}
	return git.RemoteDefaultBranch(repo)
}

// rootsInFlight counts roots, never their children: a root is in flight while
// any of its tickets is claimed or its feature branch has not reached trunk.
// An unresolvable trunk counts the branch as not landed.
func rootsInFlight(epics []tickets.Epic, repo, trunk string) int {
	n := 0
	for _, e := range epics {
		if e.Status == string(schema.StatusCancelled) {
			continue
		}
		claimed := slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool {
			return t.Status == string(schema.StatusClaimed)
		})
		if claimed || (repo != "" && branchUnlanded(repo, e.Name, trunk)) {
			n++
		}
	}
	return n
}

func branchUnlanded(repo, branch, trunk string) bool {
	if _, err := git.RevParse(repo, "refs/heads/"+branch); err != nil {
		return false
	}
	landed, err := git.IsAncestor(repo, "refs/heads/"+branch, trunk)
	return err != nil || !landed
}

// projectDirFor resolves a bare project name, or with none the cwd's project.
func projectDirFor(d deps, name string) (string, error) {
	cfg, err := d.loadConfig()
	if err != nil {
		return "", err
	}
	if name != "" {
		return tickets.ProjectDirByName(cfg.TicketStore.Path, name)
	}
	cwd, err := d.getwd()
	if err != nil {
		return "", err
	}
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return "", tickets.ErrNoProject
	}
	return tickets.ProjectDir(cfg.TicketStore.Path, repo.Root)
}

// validateProjectConfig accepts what the server's loader accepts, with known
// enum values and a well-formed trunk. An empty trunk means unset. Whether the
// trunk branch exists is not checked: it may be set before the branch is created.
func validateProjectConfig(data []byte) error {
	pf, err := config.ParseProjectFile(data)
	if err != nil {
		return err
	}
	if err := config.ValidateProjectFile(pf); err != nil {
		return err
	}
	if pf.Trunk != nil && *pf.Trunk != "" && !git.ValidBranchName(*pf.Trunk) {
		return fmt.Errorf("trunk: %q is not a valid branch name", *pf.Trunk)
	}
	return nil
}

// askEditAgain defaults to yes; end of input means no.
func askEditAgain(in *bufio.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Edit again? [Y/n] ")
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	}
	return false
}
