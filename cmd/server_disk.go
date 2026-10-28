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
)

func newServerDiskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "disk", Short: "disk space gx watches", Args: cobra.NoArgs}
	var jsonOut bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the disk-space thresholds and every disk gx watches",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := serverClient()
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
	return cmd
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
