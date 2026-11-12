package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/settingsapi"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

func newServerSettingsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "settings",
		Short: "Show the settings and their values, globally or for one project",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			target := settingsapi.GlobalTarget
			if project != "" {
				target = project
			}
			return runServerSettings(c.Context(), cl, c.OutOrStdout(), target, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "show one project's settings (default: the global ones)")
	return c
}

func runServerSettings(ctx context.Context, cl *apiclient.Client, w io.Writer, target string, jsonOut bool) error {
	resp, err := cl.Admin.Settings(ctx, target)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, resp, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, e := range resp.Entries {
		switch {
		case !e.Set:
			fmt.Fprintf(w, "%s: not set\n", e.Key)
		case len(e.Value) == 0:
			fmt.Fprintf(w, "%s: set\n", e.Key)
		default:
			fmt.Fprintf(w, "%s: %s\n", e.Key, e.Value)
		}
	}
	return nil
}

func newServerSettingsSetCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	var sets, removes []string
	c := &cobra.Command{
		Use:   "settings-set",
		Short: "Change settings, globally or for one project, together or not at all",
		Long: "Each --set takes KEY=VALUE; VALUE is JSON, or plain text taken as a string. " +
			"Each --remove takes a KEY. Both repeat. " +
			"A refused value exits 1; with --json the body is still printed first.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			req := wire.SettingsWriteRequest{Project: project}
			for _, s := range sets {
				key, val, ok := strings.Cut(s, "=")
				if !ok || key == "" {
					return fmt.Errorf("--set %q: want KEY=VALUE", s)
				}
				req.Changes = append(req.Changes, wire.SettingChange{Key: key, Value: jsonValue(val)})
			}
			for _, key := range removes {
				req.Changes = append(req.Changes, wire.SettingChange{Key: key, Remove: true})
			}
			if len(req.Changes) == 0 {
				return fmt.Errorf("nothing to change: pass --set KEY=VALUE or --remove KEY")
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runServerSettingsSet(c.Context(), cl, c.OutOrStdout(), req, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "change one project's settings (default: the global ones)")
	c.Flags().StringArrayVar(&sets, "set", nil, "KEY=VALUE to set (repeatable)")
	c.Flags().StringArrayVar(&removes, "remove", nil, "KEY to remove (repeatable)")
	return c
}

// jsonValue keeps v when it is valid JSON and quotes it as a string otherwise.
func jsonValue(v string) json.RawMessage {
	if json.Valid([]byte(v)) {
		return json.RawMessage(v)
	}
	b, _ := json.Marshal(v)
	return b
}

func runServerSettingsSet(ctx context.Context, cl *apiclient.Client, w io.Writer, req wire.SettingsWriteRequest, jsonOut bool) error {
	res, err := cl.Admin.SetSettings(ctx, req)
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
			return err
		}
		if res.Refused {
			// The body already says why; the exit code lets scripts skip parsing it.
			return &cmdbase.ExitError{Code: 1}
		}
		return nil
	}
	if res.Refused {
		return fmt.Errorf("not saved: %s: %s", strings.Join(res.Keys, ", "), res.Reason)
	}
	for _, c := range req.Changes {
		if c.Remove {
			fmt.Fprintf(w, "%s: removed\n", c.Key)
		} else {
			fmt.Fprintf(w, "%s: %s\n", c.Key, c.Value)
		}
	}
	for _, k := range res.RestartNeeded {
		fmt.Fprintf(w, "restart needed for %s\n", k)
	}
	for _, n := range res.Notices {
		fmt.Fprintln(w, n)
	}
	return nil
}
