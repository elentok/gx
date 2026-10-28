package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
)

func newServerDiskCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{Use: "disk", Short: "disk space gx watches", Args: cobra.NoArgs}
	var jsonOut bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the disk-space thresholds and every disk gx watches",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runDiskStatus(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	status.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	cmd.AddCommand(status)

	var cleanJSON bool
	clean := &cobra.Command{
		Use:   "clean <disk>",
		Short: "Run the cleanup of a disk now (disk id or name)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runDiskClean(ctx, cl, c.OutOrStdout(), args[0], cleanJSON)
		},
	}
	clean.Flags().BoolVar(&cleanJSON, "json", false, "emit the structured result as JSON")
	cmd.AddCommand(clean)
	return cmd
}

// runDiskClean runs a clean and prints its result. A refusal and a failed clean
// both return an error, so the command exits non-zero.
func runDiskClean(ctx context.Context, cl *apiclient.Client, w io.Writer, disk string, jsonOut bool) error {
	id := disk
	if rep, err := cl.DiskStatus(ctx); err == nil {
		for _, d := range rep.Disks {
			if d.Name == disk {
				id = d.ID
			}
		}
	}
	res, err := cl.CleanDisk(ctx, id)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.CleanNowResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	} else if !res.Refused {
		held := "not held"
		if res.Held {
			held = "held"
		}
		if _, err := fmt.Fprintf(w, "freed %s, %s free now, dispatch %s\n",
			config.ByteSize(res.FreedBytes).String(), config.ByteSize(res.FreeBytes).String(), held); err != nil {
			return err
		}
	}
	switch {
	case res.Refused:
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	case res.Failure != "":
		return fmt.Errorf("clean failed: %s", res.Failure)
	}
	return nil
}

func runDiskStatus(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	rep, err := cl.DiskStatus(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, rep, viaServer, callerActor(os.Getwd))
	}
	now := time.Now()
	lines := []string{fmt.Sprintf("warn below %s, clean below %s", rep.WarnBelow, rep.CleanBelow)}
	for _, d := range rep.Disks {
		line := fmt.Sprintf("%s: %s free, read %s ago, projects: %s", d.Name,
			config.ByteSize(d.FreeBytes).String(), now.Sub(d.ReadAt).Round(time.Second), strings.Join(d.Projects, ", "))
		if d.Warned {
			line += " [warned]"
		}
		lines = append(lines, line)
	}
	_, err = fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}
