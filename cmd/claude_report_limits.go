package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
)

// limitsReportFromPayload extracts the two windows from a statusline payload.
// ok is false when the payload has no usable window, so nothing is sent.
func limitsReportFromPayload(payload []byte) (report wire.LimitsReport, ok bool, err error) {
	rl, err := parseStatuslineRateLimits(payload)
	if err != nil || rl == nil {
		return report, false, err
	}
	if w := rl.FiveHour; w != nil {
		report.FiveHour = &wire.LimitWindow{UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt}
	}
	if w := rl.SevenDay; w != nil {
		report.SevenDay = &wire.LimitWindow{UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt}
	}
	return report, report.FiveHour != nil || report.SevenDay != nil, nil
}

// reportLimits sends the payload's windows to the server. A payload without a
// usable window sends nothing (sent is false). A missing server is the
// server-not-running refusal result, as for `gx usage override`.
func reportLimits(ctx context.Context, dial func() (*apiclient.Client, error), payload []byte) (res wire.LimitsReportResult, sent bool, err error) {
	report, ok, err := limitsReportFromPayload(payload)
	if err != nil || !ok {
		return res, false, err
	}
	cl, err := dial()
	if err != nil {
		return res, false, err
	}
	res, err = cl.ReportLimits(ctx, report)
	switch {
	case apiclient.IsNotRunning(err):
		return wire.LimitsReportResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, true, nil
	case err != nil:
		return res, true, fmt.Errorf("server write failed: %w", err)
	}
	return res, true, nil
}

func newClaudeReportLimitsCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "report-limits",
		Short: "send the rate_limits of a Claude statusline payload on stdin to the server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			payload, err := io.ReadAll(c.InOrStdin())
			if err != nil {
				return err
			}
			return runReportLimits(d.dial(), c, payload, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runReportLimits(dial func() (*apiclient.Client, error), c *cobra.Command, payload []byte, jsonOut bool) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	res, sent, err := reportLimits(ctx, dial, payload)
	if err != nil || !sent {
		return err
	}
	w := c.OutOrStdout()
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	return nil
}
