package serverapi

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

const commitsUnavailableText = "commits not available: a SHA of this range is not in the clone"

// newDiffCmd builds one Diff verb. The refs and path are positional so a value
// after `--` (such as `--output=x`) reaches the server and fails there.
func newDiffCmd[T any](d Deps, use, short string, args int, withCommits bool, fetch func(context.Context, *apiclient.Client, string, []string, []string) (T, error), text func(io.Writer, T) error) *cobra.Command {
	var jsonOut bool
	var project string
	var commits []string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(args),
		RunE: func(c *cobra.Command, a []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (T, error) {
					return fetch(ctx, cl, project, a, commits)
				}, text)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().StringVar(&project, "project", "", "the project whose repo to diff")
	_ = cmd.MarkFlagRequired("project")
	if withCommits {
		cmd.Flags().StringSliceVar(&commits, "commits", nil, "narrow to these commit SHAs")
	}
	return cmd
}

func newServerDiffFilesCmd(d Deps) *cobra.Command {
	return newDiffCmd(d, "diff-files <first> <second>", "list the files the second ref changed since its merge base with the first", 2, true,
		func(ctx context.Context, cl *apiclient.Client, project string, a, commits []string) (wire.DiffFiles, error) {
			return cl.Reads.DiffFiles(ctx, project, a[0], a[1], commits)
		}, printDiffFiles)
}

func newServerDiffPatchCmd(d Deps) *cobra.Command {
	return newDiffCmd(d, "diff-patch <first> <second> <path>", "show one file's patch of the range", 3, true,
		func(ctx context.Context, cl *apiclient.Client, project string, a, commits []string) (wire.DiffPatch, error) {
			return cl.Reads.DiffPatch(ctx, project, a[0], a[1], a[2], commits)
		}, printDiffPatch)
}

func newServerDiffCommitsCmd(d Deps) *cobra.Command {
	return newDiffCmd(d, "diff-commits <first> <second>", "list the range's commits with their ticket addresses", 2, false,
		func(ctx context.Context, cl *apiclient.Client, project string, a, _ []string) (wire.DiffCommits, error) {
			return cl.Reads.DiffCommits(ctx, project, a[0], a[1])
		}, printDiffCommits)
}

func newServerDiffBlobCmd(d Deps) *cobra.Command {
	return newDiffCmd(d, "diff-blob <merge-base> <tip> <path>", "read one file's old and new content by the SHAs diff-files printed", 3, false,
		func(ctx context.Context, cl *apiclient.Client, project string, a, _ []string) (wire.DiffBlobs, error) {
			return cl.Reads.DiffBlob(ctx, project, a[0], a[1], a[2])
		}, printDiffBlob)
}

func newServerDiffWorktreeCmd(d Deps) *cobra.Command {
	return newDiffCmd(d, "diff-worktree <kind>:<id>", "snapshot a chat's or Design's worktree and print the SHAs to diff", 1, false,
		func(ctx context.Context, cl *apiclient.Client, project string, a, _ []string) (wire.DiffWorktree, error) {
			return cl.Reads.DiffWorktree(ctx, project, a[0])
		}, printDiffWorktree)
}

func printDiffWorktree(w io.Writer, t wire.DiffWorktree) error {
	if t.Reason != "" {
		_, err := fmt.Fprintf(w, "no worktree diff: %s\n", t.Reason)
		return err
	}
	_, err := fmt.Fprintf(w, "base %s (%s)\nhead %s\nsnapshot %s\nuncommitted %t\nleft out %d\n",
		t.BaseLabel, t.BaseSHA, t.HeadSHA, t.SnapshotSHA, t.Uncommitted, t.LeftOut)
	return err
}

func printDiffFiles(w io.Writer, f wire.DiffFiles) error {
	if f.CommitsUnavailable {
		_, err := fmt.Fprintln(w, commitsUnavailableText)
		return err
	}
	if f.NoCommonHistory {
		_, err := fmt.Fprintln(w, "no common history")
		return err
	}
	if _, err := fmt.Fprintf(w, "merge-base %s tip %s\n", f.MergeBase, f.SecondSHA); err != nil {
		return err
	}
	for _, e := range f.Files {
		path := e.Path
		if e.OldPath != "" {
			path = e.OldPath + " -> " + e.Path
		}
		if _, err := fmt.Fprintf(w, "%s\t+%d\t-%d\t%s\n", e.Status, e.Added, e.Deleted, path); err != nil {
			return err
		}
	}
	if f.Truncated {
		_, err := fmt.Fprintln(w, "(list truncated)")
		return err
	}
	return nil
}

func printDiffPatch(w io.Writer, p wire.DiffPatch) error {
	if p.CommitsUnavailable {
		_, err := fmt.Fprintln(w, commitsUnavailableText)
		return err
	}
	if _, err := io.WriteString(w, p.Patch); err != nil {
		return err
	}
	for _, c := range p.CommitPatches {
		if _, err := fmt.Fprintf(w, "# %s %s\n%s", c.SHA, c.Subject, c.Patch); err != nil {
			return err
		}
	}
	return nil
}

func printDiffCommits(w io.Writer, c wire.DiffCommits) error {
	if c.CommitsUnavailable {
		_, err := fmt.Fprintln(w, commitsUnavailableText)
		return err
	}
	for _, e := range c.Commits {
		ticket := e.Ticket
		if ticket == "" {
			ticket = "-"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", ticket, e.SHA, e.Subject); err != nil {
			return err
		}
	}
	return nil
}

func printDiffBlob(w io.Writer, b wire.DiffBlobs) error {
	if b.CommitsUnavailable {
		_, err := fmt.Fprintln(w, commitsUnavailableText)
		return err
	}
	for _, side := range []struct {
		name string
		blob wire.DiffBlob
	}{{"old", b.Old}, {"new", b.New}} {
		line := fmt.Sprintf("%s: %s", side.name, side.blob.Kind)
		if side.blob.Size > 0 {
			line += fmt.Sprintf(" (%d bytes)", side.blob.Size)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
		if side.blob.Kind == wire.DiffBlobText {
			if _, err := fmt.Fprintln(w, strings.TrimSuffix(side.blob.Text, "\n")); err != nil {
				return err
			}
		}
	}
	return nil
}
