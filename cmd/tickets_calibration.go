package cmd

import (
	"fmt"
	"io"
	"sort"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketspec"
	"github.com/spf13/cobra"
)

// defaultCalibrationRatio is the estimate multiplier used when an epic has no
// finished tickets to measure.
const defaultCalibrationRatio = 0.83

// CalibrationRow is one counted ticket.
type CalibrationRow struct {
	ID       string  `json:"id"`
	Expected int     `json:"expected"`
	Work     int     `json:"work"`
	Ratio    float64 `json:"ratio"`
}

// Calibration is what `gx tickets calibration` reports for one epic.
type Calibration struct {
	Epic string `json:"epic"`
	// Ratio is the median work-to-expected ratio; defaultCalibrationRatio when
	// Count is 0.
	Ratio float64 `json:"ratio"`
	Count int     `json:"count"`
	// Approximate is true when any counted ticket used the fallback prefix.
	Approximate bool             `json:"approximate"`
	Tickets     []CalibrationRow `json:"tickets"`
}

// buildCalibration counts the epic's done implement tickets that have both
// numbers set. Stub windows, commitless tickets and zero work are skipped.
func buildCalibration(name string, e tickets.Epic) Calibration {
	c := Calibration{Epic: name, Ratio: defaultCalibrationRatio, Tickets: []CalibrationRow{}}
	var ratios []float64
	for _, tk := range e.Tickets {
		if tk.Status != "done" || tk.Type != "implement" || tk.Commitless ||
			tk.ExpectedContextWindow <= 0 || tk.ActualContextWindow <= 0 {
			continue
		}
		work, approx := ticketspec.Work(tk)
		if work == 0 {
			continue
		}
		ratio := float64(work) / float64(tk.ExpectedContextWindow)
		c.Tickets = append(c.Tickets, CalibrationRow{ID: tk.Identifier, Expected: tk.ExpectedContextWindow, Work: work, Ratio: ratio})
		ratios = append(ratios, ratio)
		c.Approximate = c.Approximate || approx
	}
	c.Count = len(ratios)
	if c.Count > 0 {
		sort.Float64s(ratios)
		mid := c.Count / 2
		c.Ratio = ratios[mid]
		if c.Count%2 == 0 {
			c.Ratio = (ratios[mid-1] + ratios[mid]) / 2
		}
	}
	return c
}

func runTicketsCalibration(cfgDirs config.Dirs, cwd, arg string, asJSON bool, w io.Writer) error {
	projectDir, project, name, err := resolveEpicTarget(cfgDirs, cwd, arg)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return err
	}
	for _, e := range epics {
		if e.Name == name {
			return writeCalibration(w, buildCalibration(project+":"+name, e), asJSON)
		}
	}
	return &tickets.AddressError{Code: tickets.CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q in project %q", name, project)}
}

func writeCalibration(w io.Writer, c Calibration, asJSON bool) error {
	if asJSON {
		return cmdbase.WriteJSON(w, c)
	}
	if c.Count == 0 {
		_, err := fmt.Fprintf(w, "no finished tickets, use %.2f\n", defaultCalibrationRatio)
		return err
	}
	fmt.Fprintf(w, "median work/expected ratio: %.2f\nfinished tickets: %d\n", c.Ratio, c.Count)
	if c.Approximate {
		fmt.Fprintln(w, "approximate: some tickets used the fallback prefix")
	}
	for _, r := range c.Tickets {
		fmt.Fprintf(w, "%s expected=%d work=%d\n", r.ID, r.Expected, r.Work)
	}
	return nil
}

func newTicketsCalibrationCmd(d deps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "calibration <project:epic>",
		Short: "print the epic's median work-to-expected ratio over its finished implement tickets",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsCalibration(d.dirs(), cwd, args[0], asJSON, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit structured JSON instead of text")
	return cmd
}
