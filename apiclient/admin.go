package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/cpuwatch"
	"github.com/elentok/gx/server/wire"
)

// Admin is the sub-client for config, checks, harness, health, restart,
// repair, disk, limits and CPU calls. It shares Client's transport.
type Admin struct {
	t *transport
}

// BudgetOverride lifts the budget latches. A refusal is a result (Refused set),
// not an error.
func (a *Admin) BudgetOverride(ctx context.Context) (wire.BudgetResult, error) {
	var res wire.BudgetResult
	err := a.t.post(ctx, "/v1/budget/override", struct{}{}, &res)
	return res, err
}

// UsageOverride lifts one harness's usage gate until the gating windows reset;
// an empty harness means the Default harness. A refusal is a result (Refused
// set), not an error.
func (a *Admin) UsageOverride(ctx context.Context, harness string) (wire.UsageOverrideResult, error) {
	var res wire.UsageOverrideResult
	err := a.t.post(ctx, "/v1/usage/override", wire.UsageOverrideRequest{Harness: harness}, &res)
	return res, err
}

// ReportLimits sends Claude rate-limit windows and returns the windows the
// server stores after the merge. A report with no windows reads them back. A
// refusal is a result (Refused set), not an error.
func (a *Admin) ReportLimits(ctx context.Context, report wire.LimitsReport) (wire.LimitsReportResult, error) {
	var res wire.LimitsReportResult
	err := a.t.post(ctx, "/v1/usage/report", report, &res)
	return res, err
}

// ReloadConfig reloads the global config file into the server. A refusal is
// a result (Refused set), not an error.
func (a *Admin) ReloadConfig(ctx context.Context) (wire.ReloadResult, error) {
	var res wire.ReloadResult
	err := a.t.post(ctx, "/v1/config/reload", struct{}{}, &res)
	return res, err
}

// ConfigStatus is the last reload result, the startup-only keys waiting for a
// restart and the hard-limit grace countdown.
func (a *Admin) ConfigStatus(ctx context.Context) (wire.ConfigStatus, error) {
	var st wire.ConfigStatus
	err := a.t.get(ctx, "/v1/config/status", &st)
	return st, err
}

// RestartCheck runs the restart check from the binary a restart would start.
func (a *Admin) RestartCheck(ctx context.Context) (wire.RestartCheckReport, error) {
	var rep wire.RestartCheckReport
	err := a.t.get(ctx, "/v1/restart/check", &rep)
	return rep, err
}

// Restart starts a restart when the check still matches req. A refusal is a
// result (Refused set), not an error.
func (a *Admin) Restart(ctx context.Context, req wire.RestartRequest) (wire.RestartResult, error) {
	var res wire.RestartResult
	err := a.t.post(ctx, "/v1/restart", req, &res)
	return res, err
}

// CleanDisk runs the cleanup of one disk now. A refusal is a result (Refused
// set), not an error.
func (a *Admin) CleanDisk(ctx context.Context, disk string) (wire.CleanNowResult, error) {
	var res wire.CleanNowResult
	err := a.t.post(ctx, "/v1/disk/clean", wire.CleanNowRequest{Disk: disk}, &res)
	return res, err
}

// DiskStatus is the disk-space thresholds and every disk the watcher knows.
func (a *Admin) DiskStatus(ctx context.Context) (wire.DiskReport, error) {
	var rep wire.DiskReport
	err := a.t.get(ctx, "/v1/disk/status", &rep)
	return rep, err
}

// SaveChecks saves one project's check config to the repo or project.json. A
// refusal is a result (Refused set), not an error.
func (a *Admin) SaveChecks(ctx context.Context, req wire.SaveChecksRequest) (wire.SaveChecksResult, error) {
	var res wire.SaveChecksResult
	err := a.t.post(ctx, "/v1/checks/save", req, &res)
	return res, err
}

// DefaultHarness is the global Default harness, the agent runner and each
// project's override.
func (a *Admin) DefaultHarness(ctx context.Context) (wire.DefaultHarnessStatus, error) {
	var st wire.DefaultHarnessStatus
	err := a.t.get(ctx, "/v1/settings/agents", &st)
	return st, err
}

// SetDefaultHarness writes the Default harness. A refusal is a result (Refused
// set), not an error.
func (a *Admin) SetDefaultHarness(ctx context.Context, req wire.SetDefaultHarnessRequest) (wire.SetDefaultHarnessResult, error) {
	var res wire.SetDefaultHarnessResult
	err := a.t.post(ctx, "/v1/settings/agents", req, &res)
	return res, err
}

// ChecksStatus is each project's check config and detected commands. An empty
// project means every registered project.
func (a *Admin) ChecksStatus(ctx context.Context, project string) (wire.ChecksStatus, error) {
	var st wire.ChecksStatus
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	err := a.t.get(ctx, "/v1/checks/status?"+q.Encode(), &st)
	return st, err
}

// Health is the machine's health: host, CPU, memory and every watched disk.
func (a *Admin) Health(ctx context.Context) (wire.HealthReport, error) {
	var rep wire.HealthReport
	err := a.t.get(ctx, "/v1/health", &rep)
	return rep, err
}

// HealthWait is Health with the `wait` option: a CPU list still reading is
// held back until the server's second process read ends.
func (a *Admin) HealthWait(ctx context.Context) (wire.HealthReport, error) {
	var rep wire.HealthReport
	err := a.t.get(ctx, "/v1/health?wait", &rep)
	return rep, err
}

// BudgetIncrease raises the live budget limits until the server restarts. A
// refusal is a result (Refused set), not an error.
func (a *Admin) BudgetIncrease(ctx context.Context, req wire.BudgetIncreaseRequest) (wire.BudgetResult, error) {
	var res wire.BudgetResult
	err := a.t.post(ctx, "/v1/budget/increase", req, &res)
	return res, err
}

// Repair runs a repair verb ("merge", "start-over", "resume" or "verify"). A refusal
// is a result (Refused set), not an error.
func (a *Admin) Repair(ctx context.Context, verb string, req wire.RepairRequest) (wire.RepairResult, error) {
	var res wire.RepairResult
	err := a.t.post(ctx, "/v1/tickets/"+verb, req, &res)
	return res, err
}

// CPUSpike reports a TUI's CPU spike; the server decides whether to file a ticket.
func (a *Admin) CPUSpike(ctx context.Context, spike cpuwatch.Spike) (wire.CPUSpikeResult, error) {
	var res wire.CPUSpikeResult
	err := a.t.post(ctx, "/v1/cpu-spike", spike, &res)
	return res, err
}
