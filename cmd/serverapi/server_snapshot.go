package serverapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// KindInfo is one entry of the `gx server events kinds --json` payload.
type KindInfo struct {
	Kind       string `json:"kind"`
	CauseHerdr bool   `json:"cause_herdr"`
}

func newServerSnapshotCmd(d Deps) *cobra.Command {
	var snapJSON bool
	snapshot := &cobra.Command{
		Use:   "snapshot",
		Short: "print the server's ticket index and sequence number",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerSnapshot(c.Context(), d.Dirs(), snapJSON, c.OutOrStdout())
		},
	}
	snapshot.Flags().BoolVar(&snapJSON, "json", false, "emit structured JSON instead of human-readable text")
	return snapshot
}

func newServerEventsCmd(d Deps) *cobra.Command {
	eventsCmd := &cobra.Command{
		Use:   "events",
		Short: "run-log event contract",
		Args:  cobra.NoArgs,
	}
	var jsonOut bool
	kinds := &cobra.Command{
		Use:   "kinds",
		Short: "print the closed event kind enum (no server needed)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServerEventsKinds(jsonOut, c.OutOrStdout())
		},
	}
	kinds.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	eventsCmd.AddCommand(kinds)
	return eventsCmd
}

func runServerSnapshot(ctx context.Context, dirs config.Dirs, jsonOut bool, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c, err := cmdbase.ServerClient(dirs)
	if err != nil {
		return err
	}
	snap, err := c.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("server not reachable (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, snap, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if _, err := fmt.Fprintf(w, "seq: %d\n", snap.Seq); err != nil {
		return err
	}
	for _, t := range snap.Tickets {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", t.Address, t.Status, t.Title); err != nil {
			return err
		}
	}
	return nil
}

// runServerTicketsFollow prints addr's state on every (re-)snapshot where it
// changed, and each event naming addr. Other tickets' events are dropped.
// With jsonOut each output line is one JSON object (events, and ticket state
// or {"address":...,"absent":true}).
func runServerTicketsFollow(ctx context.Context, c *apiclient.Client, addr string, jsonOut bool, w io.Writer) error {
	var last string
	var werr error
	print := func(text string, v any) {
		if werr != nil {
			return
		}
		if jsonOut {
			werr = json.NewEncoder(w).Encode(v)
			return
		}
		_, werr = fmt.Fprintln(w, text)
	}
	err := c.Follow(ctx, func(s *wire.Snapshot, ev *wire.Event) {
		if ev != nil {
			if ev.Address == addr {
				print(fmt.Sprintf("%d\t%s\t%s", ev.Seq, ev.Type, ev.Address), ev)
			}
			return
		}
		line, state := "absent", any(map[string]any{"address": addr, "absent": true})
		for _, t := range s.Tickets {
			if t.Address == addr {
				line, state = fmt.Sprintf("%s\t%s\t%s", t.Address, t.Status, t.Title), t
			}
		}
		if line != last {
			last = line
			print(line, state)
		}
	})
	if werr != nil {
		return werr
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// runServerEventsKinds prints every event kind, reading Go data only.
func runServerEventsKinds(jsonOut bool, w io.Writer) error {
	all := events.Kinds()
	infos := make([]KindInfo, 0, len(all))
	for _, k := range all {
		infos = append(infos, KindInfo{Kind: string(k), CauseHerdr: k.CauseHerdr()})
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(infos)
	}
	for _, i := range infos {
		line := i.Kind
		if i.CauseHerdr {
			line += "\tcause: herdr"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
