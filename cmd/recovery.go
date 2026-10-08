package cmd

import (
	"encoding/json"
	"io"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/recovery"
	"github.com/spf13/cobra"
)

// CatalogInfo is the `gx recovery catalog --json` payload.
type CatalogInfo struct {
	Enabled       bool              `json:"enabled"`
	Entries       []recovery.Entry  `json:"entries"`
	NotCatalogued map[string]string `json:"not_catalogued"`
}

func newRecoveryCmd(_ deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery",
		Short: "recovery catalog commands",
		Args:  cobra.NoArgs,
	}
	var jsonOut bool
	catalog := &cobra.Command{
		Use:   "catalog",
		Short: "print the recovery catalog with the user's kill switch and disables applied",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cat := recovery.Default().WithConfig(cfg.Recovery.Enabled, cfg.Recovery.Disabled)
			return runRecoveryCatalog(cat, jsonOut, c.OutOrStdout())
		},
	}
	catalog.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(catalog)
	return cmd
}

func runRecoveryCatalog(cat recovery.Catalog, jsonOut bool, w io.Writer) error {
	info := CatalogInfo{Enabled: cat.Enabled, Entries: cat.Entries, NotCatalogued: recovery.NotCatalogued}
	if info.Entries == nil {
		info.Entries = []recovery.Entry{}
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	state := "enabled"
	if !cat.Enabled {
		state = "disabled (kill switch)"
	}
	if _, err := io.WriteString(w, "recovery: "+state+"\n"); err != nil {
		return err
	}
	for _, e := range info.Entries {
		on := "on"
		if !e.Enabled {
			on = "off"
		}
		if _, err := io.WriteString(w, e.ID+"\t"+string(e.Type)+"/"+string(e.Kind)+"\t"+string(e.Executor)+"\t"+string(e.Authority)+"\t"+on+"\n"); err != nil {
			return err
		}
	}
	return nil
}
