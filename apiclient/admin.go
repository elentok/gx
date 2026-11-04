package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// SetAFK sets AFK mode; a nil on toggles it.
func (c *Client) SetAFK(ctx context.Context, on *bool) (server.AFKResult, error) {
	var res server.AFKResult
	err := c.post(ctx, "/v1/afk", server.AFKRequest{On: on}, &res)
	return res, err
}

// SetViMode sets the web UI's vi mode; a nil on toggles it.
func (c *Client) SetViMode(ctx context.Context, on *bool) (server.ViModeResult, error) {
	var res server.ViModeResult
	err := c.post(ctx, "/v1/vimode", server.ViModeRequest{On: on}, &res)
	return res, err
}

// BudgetOverride lifts the budget latches. A refusal is a result (Refused set),
// not an error.
func (c *Client) BudgetOverride(ctx context.Context) (server.BudgetResult, error) {
	var res server.BudgetResult
	err := c.post(ctx, "/v1/budget/override", struct{}{}, &res)
	return res, err
}

// UsageOverride lifts one harness's usage gate until the gating windows reset;
// an empty harness means the Default harness. A refusal is a result (Refused
// set), not an error.
func (c *Client) UsageOverride(ctx context.Context, harness string) (server.UsageOverrideResult, error) {
	var res server.UsageOverrideResult
	err := c.post(ctx, "/v1/usage/override", server.UsageOverrideRequest{Harness: harness}, &res)
	return res, err
}

// ReportLimits sends Claude rate-limit windows and returns the windows the
// server stores after the merge. A report with no windows reads them back. A
// refusal is a result (Refused set), not an error.
func (c *Client) ReportLimits(ctx context.Context, report server.LimitsReport) (server.LimitsReportResult, error) {
	var res server.LimitsReportResult
	err := c.post(ctx, "/v1/usage/report", report, &res)
	return res, err
}

// ReloadConfig reloads the global config file into the server. A refusal is
// a result (Refused set), not an error.
func (c *Client) ReloadConfig(ctx context.Context) (server.ReloadResult, error) {
	var res server.ReloadResult
	err := c.post(ctx, "/v1/config/reload", struct{}{}, &res)
	return res, err
}

// ConfigStatus is the last reload result, the startup-only keys waiting for a
// restart and the hard-limit grace countdown.
func (c *Client) ConfigStatus(ctx context.Context) (server.ConfigStatus, error) {
	var st server.ConfigStatus
	err := c.get(ctx, "/v1/config/status", &st)
	return st, err
}

// RestartCheck runs the restart check from the binary a restart would start.
func (c *Client) RestartCheck(ctx context.Context) (wire.RestartCheckReport, error) {
	var rep wire.RestartCheckReport
	err := c.get(ctx, "/v1/restart/check", &rep)
	return rep, err
}

// Restart starts a restart when the check still matches req. A refusal is a
// result (Refused set), not an error.
func (c *Client) Restart(ctx context.Context, req wire.RestartRequest) (wire.RestartResult, error) {
	var res wire.RestartResult
	err := c.post(ctx, "/v1/restart", req, &res)
	return res, err
}

// CleanDisk runs the cleanup of one disk now. A refusal is a result (Refused
// set), not an error.
func (c *Client) CleanDisk(ctx context.Context, disk string) (wire.CleanNowResult, error) {
	var res wire.CleanNowResult
	err := c.post(ctx, "/v1/disk/clean", wire.CleanNowRequest{Disk: disk}, &res)
	return res, err
}

// DiskStatus is the disk-space thresholds and every disk the watcher knows.
func (c *Client) DiskStatus(ctx context.Context) (wire.DiskReport, error) {
	var rep wire.DiskReport
	err := c.get(ctx, "/v1/disk/status", &rep)
	return rep, err
}

// SaveChecks saves one project's check config to the repo or project.json. A
// refusal is a result (Refused set), not an error.
func (c *Client) SaveChecks(ctx context.Context, req server.SaveChecksRequest) (server.SaveChecksResult, error) {
	var res server.SaveChecksResult
	err := c.post(ctx, "/v1/checks/save", req, &res)
	return res, err
}

// DefaultHarness is the global Default harness, the agent runner and each
// project's override.
func (c *Client) DefaultHarness(ctx context.Context) (server.DefaultHarnessStatus, error) {
	var st server.DefaultHarnessStatus
	err := c.get(ctx, "/v1/settings/agents", &st)
	return st, err
}

// SetDefaultHarness writes the Default harness. A refusal is a result (Refused
// set), not an error.
func (c *Client) SetDefaultHarness(ctx context.Context, req server.SetDefaultHarnessRequest) (server.SetDefaultHarnessResult, error) {
	var res server.SetDefaultHarnessResult
	err := c.post(ctx, "/v1/settings/agents", req, &res)
	return res, err
}

// ChecksStatus is each project's check config and detected commands. An empty
// project means every registered project.
func (c *Client) ChecksStatus(ctx context.Context, project string) (wire.ChecksStatus, error) {
	var st wire.ChecksStatus
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	err := c.get(ctx, "/v1/checks/status?"+q.Encode(), &st)
	return st, err
}

// Health is the machine's health: host, CPU, memory and every watched disk.
func (c *Client) Health(ctx context.Context) (wire.HealthReport, error) {
	var rep wire.HealthReport
	err := c.get(ctx, "/v1/health", &rep)
	return rep, err
}

// HealthWait is Health with the `wait` option: a CPU list still reading is
// held back until the server's second process read ends.
func (c *Client) HealthWait(ctx context.Context) (wire.HealthReport, error) {
	var rep wire.HealthReport
	err := c.get(ctx, "/v1/health?wait", &rep)
	return rep, err
}

// BudgetIncrease raises the live budget limits until the server restarts. A
// refusal is a result (Refused set), not an error.
func (c *Client) BudgetIncrease(ctx context.Context, req server.BudgetIncreaseRequest) (server.BudgetResult, error) {
	var res server.BudgetResult
	err := c.post(ctx, "/v1/budget/increase", req, &res)
	return res, err
}

// Repair runs a repair verb ("merge", "start-over", "resume" or "verify"). A refusal
// is a result (Refused set), not an error.
func (c *Client) Repair(ctx context.Context, verb string, req server.RepairRequest) (server.RepairResult, error) {
	var res server.RepairResult
	err := c.post(ctx, "/v1/tickets/"+verb, req, &res)
	return res, err
}
