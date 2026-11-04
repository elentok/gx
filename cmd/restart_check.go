package cmd

import (
	"encoding/json"
	"io"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// restartCheck is the JSON "gx restart-check" prints. The server runs it from
// the binary a restart would start: a binary without the command exits non-zero
// with an unknown-command error, one that ran exits zero even when the config
// or a dry run failed (the failure is in the JSON).
type (
	restartCheck        = wire.RestartCheckOutput
	restartCheckConfig  = wire.RestartCheckConfig
	restartCheckProject = wire.RestartCheckProject
)

func newRestartCheckCmd(d deps) *cobra.Command {
	var configOnly bool
	cmd := &cobra.Command{
		Use:    "restart-check",
		Short:  "print this binary's version, config check and migration dry run as JSON (used by the server before a restart)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runRestartCheck(d.dirs(), configOnly, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&configOnly, "config-only", false, "only load the config: print nothing and return its error")
	return cmd
}

// runRestartCheck loads the config the way runServer does, then dry-runs the
// migration on every project in the ticket store. It writes nothing. With
// configOnly a config failure is returned, so it exits non-zero with the
// startup error text on stderr.
func runRestartCheck(dirs config.Dirs, configOnly bool, w io.Writer) error {
	cfg, err := dirs.Load()
	if configOnly {
		return err
	}
	res := restartCheck{Version: getVersion(), Config: restartCheckConfig{OK: err == nil}, Projects: []restartCheckProject{}}
	if err != nil {
		res.Config.Error = err.Error()
	} else {
		res.Projects = dryRunProjects(dirs, cfg.TicketStore.Path)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func dryRunProjects(dirs config.Dirs, storePath string) []restartCheckProject {
	projects := []restartCheckProject{}
	if storePath == "" {
		return projects
	}
	dirList, err := tickets.ProjectDirs(storePath)
	if err != nil {
		return append(projects, restartCheckProject{Name: filepath.Base(storePath), Error: err.Error(), Files: []string{}})
	}
	for _, dir := range dirList {
		p := restartCheckProject{Name: tickets.ProjectName(dir), Files: []string{}}
		pending, err := migrations.Default.PendingCount(dir)
		p.Pending = pending
		// Nothing pending: skip the scratch copy.
		if err == nil && pending > 0 {
			var changes []tickets.MigrationChange
			if changes, err = dryRunChanges(dirs, dir, true); err == nil {
				for _, c := range changes {
					p.Files = append(p.Files, c.Path)
				}
			}
		}
		if err != nil {
			p.Error = err.Error()
		} else {
			p.OK = true
		}
		projects = append(projects, p)
	}
	return projects
}
