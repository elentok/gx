package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire/ops"
)

func newServerFilesRootsCmd(d Deps) *cobra.Command {
	c := &cobra.Command{Use: "files-roots", Short: "List, add and remove the extra folders gx may host files from"}
	c.AddCommand(newFilesRootsList(d), newFilesRootsAdd(d), newFilesRootsRemove(d))
	return c
}

func newFilesRootsList(d Deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List every hosted-file root, built-in and configured",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runFilesRoots(d, c, jsonOut, "server read failed", (*apiclient.Web).HostedRoots)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func newFilesRootsAdd(d Deps) *cobra.Command {
	return newFilesRootsEdit(d, "add <path>", "Add an extra folder to host files from", (*apiclient.Web).AddHostedRoot)
}

func newFilesRootsRemove(d Deps) *cobra.Command {
	return newFilesRootsEdit(d, "remove <path>", "Remove an extra hosted-file root", (*apiclient.Web).RemoveHostedRoot)
}

func newFilesRootsEdit(d Deps, use, short string, call func(*apiclient.Web, context.Context, string) (ops.HostedRootsResult, error)) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path, err := absRootPath(d, args[0])
			if err != nil {
				return err
			}
			return runFilesRoots(d, c, jsonOut, "server write failed",
				func(w *apiclient.Web, ctx context.Context) (ops.HostedRootsResult, error) { return call(w, ctx, path) })
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func runFilesRoots(d Deps, c *cobra.Command, jsonOut bool, failure string, call func(*apiclient.Web, context.Context) (ops.HostedRootsResult, error)) error {
	cl, ctx, err := webClient(d, c)
	if err != nil {
		return err
	}
	return runWebCall(ctx, func(ctx context.Context) (ops.HostedRootsResult, error) { return call(cl.Web, ctx) },
		failure, jsonOut, c.OutOrStdout(), func(res ops.HostedRootsResult) error { return printRoots(c.OutOrStdout(), res) })
}

func printRoots(w io.Writer, res ops.HostedRootsResult) error {
	for _, r := range res.Roots {
		if _, err := fmt.Fprintf(w, "%-9s %s\n", r.Source, r.Path); err != nil {
			return err
		}
	}
	return nil
}

// absRootPath makes a relative or "~/" path absolute, so the server only sees absolute ones.
func absRootPath(d Deps, p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := d.Dirs().Home()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[1:]), nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	getwd := d.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	cwd, err := getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, p), nil
}
