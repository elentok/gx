package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

func newProjectOnboardCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard [project]",
		Short: "set up a project's check config (project defaults to the current repo's; asks the server)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runProjectOnboard(c.Context(), cl, d, name)
		},
	}
}

// runProjectOnboard defaults name to the current repo's project.
func runProjectOnboard(ctx context.Context, cl *apiclient.Client, d deps, name string) error {
	if name == "" {
		dir, err := projectDirFor(d, "")
		if err != nil {
			return err
		}
		name = tickets.ProjectName(dir)
	}
	missing, err := onboardChecks(ctx, cl, d, d.stdout, name)
	if err == nil && !missing {
		fmt.Fprintln(d.stdout, "nothing missing")
	}
	return err
}

// onboardChecks reads the project's check state and, when config is missing,
// asks in a terminal. Without one it asks nothing and prints how to run it
// later. It reports whether config was missing.
func onboardChecks(ctx context.Context, cl *apiclient.Client, d deps, w io.Writer, project string) (bool, error) {
	pc, err := missingChecks(ctx, cl, project)
	if err != nil || pc == nil {
		return false, err
	}
	if !isTerminal(d) {
		fmt.Fprintf(w, "checks not configured: %s\nrun `%s` to set them up\n", strings.Join(missingKinds(*pc), ", "), onboardCommand(project))
		return true, nil
	}
	return true, askChecks(ctx, cl, d, w, project, *pc)
}

func isTerminal(d deps) bool { return d.stdinIsTerminal != nil && d.stdinIsTerminal() }

func onboardCommand(project string) string { return "gx project onboard " + project }

// missingChecks returns the project's check state, or nil when it has config.
func missingChecks(ctx context.Context, cl *apiclient.Client, project string) (*server.ProjectChecks, error) {
	st, err := cl.ChecksStatus(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	for i := range st.Projects {
		if st.Projects[i].Project == project && st.Projects[i].Missing {
			return &st.Projects[i], nil
		}
	}
	return nil, nil
}

func missingKinds(pc server.ProjectChecks) []string {
	kinds := []string{}
	for _, k := range pc.Kinds {
		if k.State != server.KindStateConfigured {
			kinds = append(kinds, k.Kind)
		}
	}
	return kinds
}

func askChecks(ctx context.Context, cl *apiclient.Client, d deps, w io.Writer, project string, pc server.ProjectChecks) error {
	in := bufio.NewReader(d.stdin)
	proposal := proposeChecks(pc)

	fmt.Fprintf(w, "Check config is missing for %s. Recommended:\n", project)
	for _, k := range pc.Kinds {
		rec := "gx default"
		if k.Recommended != nil {
			rec = fmt.Sprintf("%s  (%s)", k.Recommended.Command, k.Recommended.Source)
		}
		fmt.Fprintf(w, "  %-10s %s\n", k.Kind, rec)
	}
	choice := choose(in, w, "What now?", []string{"accept all", "edit", "keep the gx defaults", "skip"})
	switch choice {
	case 0:
	case 1:
		edited, ok, err := editChecks(d, in, w, proposal)
		if err != nil || !ok {
			return err
		}
		proposal = edited
	case 2:
		proposal = []byte("{}")
	default:
		fmt.Fprintf(w, "skipped; nothing saved. Run `%s` to set up later\n", onboardCommand(project))
		return nil
	}

	target := server.SaveTargetProjectJSON
	if slices.Contains(pc.SaveTargets, server.SaveTargetRepo) {
		repoLabel := fmt.Sprintf("commit %s on %s", check.RepoConfigPath, pc.Trunk)
		if choose(in, w, "Where to save?", []string{repoLabel, "keep it in project.json"}) == 0 {
			target = server.SaveTargetRepo
		}
	}
	res, err := cl.SaveChecks(ctx, server.SaveChecksRequest{Project: project, Target: target, Checks: proposal})
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	switch {
	case target == server.SaveTargetProjectJSON:
		fmt.Fprintf(w, "Saved checks to %s\n", res.Path)
	case res.Commit == "":
		fmt.Fprintf(w, "%s on %s already holds this config\n", check.RepoConfigPath, res.Trunk)
	default:
		fmt.Fprintf(w, "Committed %s on %s\n", check.RepoConfigPath, res.Trunk)
	}
	return nil
}

// proposeChecks turns the recommendations into checks.json: found commands as
// {"full": cmd}, kinds with nothing found left out.
func proposeChecks(pc server.ProjectChecks) []byte {
	cfg := map[string]map[string]string{}
	for _, k := range pc.Kinds {
		if k.Recommended != nil {
			cfg[k.Kind] = map[string]string{"full": k.Recommended.Command}
		}
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

// choose prints a numbered menu and returns the 0-based pick. An empty answer
// picks the first (the default); end of input picks nothing (-1); an
// unreadable answer asks again.
func choose(in *bufio.Reader, w io.Writer, title string, options []string) int {
	for {
		fmt.Fprintln(w, title)
		for i, o := range options {
			fmt.Fprintf(w, "  %d) %s\n", i+1, o)
		}
		fmt.Fprint(w, "Choice [1]: ")
		line, err := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" && err == nil {
			return 0
		}
		var n int
		if _, serr := fmt.Sscanf(line, "%d", &n); serr == nil && n >= 1 && n <= len(options) {
			return n - 1
		}
		if err != nil {
			return -1
		}
	}
}

// editChecks opens the proposal in $EDITOR until it passes check.ParseConfig.
// ok is false when the user discarded the edit.
func editChecks(d deps, in *bufio.Reader, w io.Writer, proposal []byte) (edited []byte, ok bool, err error) {
	editor := d.getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return nil, false, fmt.Errorf("$EDITOR is not set")
	}
	tmp, err := os.CreateTemp("", "gx-checks-*.json")
	if err != nil {
		return nil, false, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	_, err = tmp.Write(append(proposal, '\n'))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, false, err
	}
	for {
		if err := d.runEditor(editor, path, d.stdin, d.stdout, d.stderr); err != nil {
			return nil, false, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false, err
		}
		perr := func() error { _, e := check.ParseConfig(data); return e }()
		if perr == nil {
			return data, true, nil
		}
		fmt.Fprintf(d.stderr, "invalid checks: %v\n", perr)
		if !askEditAgain(in, w) {
			fmt.Fprintln(w, "edits discarded; nothing saved")
			return nil, false, nil
		}
	}
}
