package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
)

func newServerHealthCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show the server machine's CPU, memory and disks",
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
			return runServerHealth(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return cmd
}

func runServerHealth(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	rep, err := cl.HealthWait(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, rep, viaServer, callerActor(os.Getwd))
	}
	lines := []string{
		"host " + rep.Host,
		meterLine("cpu", rep.CPU.State, rep.CPU.Reason, rep.CPU.Percent, rep.CPU.Average),
	}
	mem := meterLine("memory", rep.Memory.State, rep.Memory.Reason, rep.Memory.Percent, rep.Memory.Average)
	if rep.Memory.TotalBytes > 0 {
		mem += fmt.Sprintf(", %s of %s", config.ByteSize(rep.Memory.UsedBytes).String(), config.ByteSize(rep.Memory.TotalBytes).String())
	}
	if rep.Memory.Pressure != "" {
		mem += ", pressure " + rep.Memory.Pressure
	}
	lines = append(lines, mem)
	for _, d := range rep.Disks.Disks {
		line := fmt.Sprintf("disk %s: %s, %s free of %s", d.Name, d.State,
			config.ByteSize(d.FreeBytes).String(), config.ByteSize(d.TotalBytes).String())
		if d.Held {
			line += " [held]"
		}
		lines = append(lines, line)
	}
	lines = append(lines, causeLines("top by cpu", rep.Causes.CPU, func(r server.CauseRow) string {
		return fmt.Sprintf("%.1f%%", r.CPU)
	})...)
	lines = append(lines, causeLines("top by memory", rep.Causes.Memory, func(r server.CauseRow) string {
		return config.ByteSize(r.MemBytes).String()
	})...)
	_, err = fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

// causeLines is one cause list; a marker takes the place of its rows.
func causeLines(title string, list server.CauseList, value func(server.CauseRow) string) []string {
	switch list.Marker {
	case server.CauseMarkerReading:
		return []string{title + ": reading…"}
	case server.CauseMarkerUnavailable:
		return []string{title + ": processes unavailable"}
	}
	lines := []string{title + ":"}
	for _, r := range list.Rows {
		label := r.Label
		if r.Busiest != "" {
			label += " · " + r.Busiest
		}
		lines = append(lines, "  "+label+" "+value(r))
	}
	return lines
}

// meterLine is one CPU or memory line; an unknown meter names its reason.
func meterLine(name string, state server.HealthState, reason server.HealthReason, pct, avg float64) string {
	if state == server.HealthStateUnknown {
		return fmt.Sprintf("%s: unknown (%s)", name, reason)
	}
	return fmt.Sprintf("%s: %s, %.0f%% now, %.0f%% average", name, state, pct, avg)
}
