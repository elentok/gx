package apiclient

import (
	"context"
	"net/url"
	"strconv"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// Projects returns every project in the server's ticket store with its state.
func (c *Client) Projects(ctx context.Context) ([]wire.ProjectInfo, error) {
	var p []wire.ProjectInfo
	err := c.get(ctx, "/v1/projects", &p)
	return p, err
}

// ProjectWorktrees returns each project's repo root and other worktree dirs.
func (c *Client) ProjectWorktrees(ctx context.Context) ([]wire.ProjectWorktrees, error) {
	var p []wire.ProjectWorktrees
	err := c.get(ctx, "/v1/projects/worktrees", &p)
	return p, err
}

// AddProject registers a project. A refusal is a result (Refused set), not an
// error.
func (c *Client) AddProject(ctx context.Context, req server.AddProjectRequest) (server.AddProjectResult, error) {
	var res server.AddProjectResult
	err := c.post(ctx, "/v1/projects/add", req, &res)
	return res, err
}

// RemoveProject unregisters a project, keeping its tickets. A refusal is a
// result (Refused set), not an error.
func (c *Client) RemoveProject(ctx context.Context, req server.ProjectRequest) (server.ProjectResult, error) {
	var res server.ProjectResult
	err := c.post(ctx, "/v1/projects/remove", req, &res)
	return res, err
}

// SetProjectPath points a project at another repo. A refusal is a result.
func (c *Client) SetProjectPath(ctx context.Context, req server.ProjectRequest) (server.ProjectResult, error) {
	var res server.ProjectResult
	err := c.post(ctx, "/v1/projects/set-path", req, &res)
	return res, err
}

// Locks returns every held lock with its owner.
func (c *Client) Locks(ctx context.Context) ([]wire.LockInfo, error) {
	var l []wire.LockInfo
	err := c.get(ctx, "/v1/locks", &l)
	return l, err
}

// Budget returns today's spend and the configured limits.
func (c *Client) Budget(ctx context.Context) (wire.BudgetStatus, error) {
	var b wire.BudgetStatus
	err := c.get(ctx, "/v1/budget", &b)
	return b, err
}

// UsageSummary returns the usage mode, the stored limit windows and the spend totals.
func (c *Client) UsageSummary(ctx context.Context) (wire.UsageSummary, error) {
	var u wire.UsageSummary
	err := c.get(ctx, "/v1/usage/summary", &u)
	return u, err
}

// History returns the events logged for one ticket, by its full address.
func (c *Client) History(ctx context.Context, address string) (wire.History, error) {
	var h wire.History
	err := c.get(ctx, "/v1/tickets/history?address="+url.QueryEscape(address), &h)
	return h, err
}

// Explain returns the scheduler's verdict on one ticket, by its full address.
func (c *Client) Explain(ctx context.Context, address string) (wire.Explanation, error) {
	var e wire.Explanation
	err := c.get(ctx, "/v1/tickets/explain?address="+url.QueryEscape(address), &e)
	return e, err
}

// Detail returns one ticket's pause detail, by its full address.
func (c *Client) Detail(ctx context.Context, address string) (wire.TicketDetail, error) {
	var d wire.TicketDetail
	err := c.get(ctx, "/v1/tickets/detail?address="+url.QueryEscape(address), &d)
	return d, err
}

// Epics returns every epic the Epics page lists.
func (c *Client) Epics(ctx context.Context) (wire.EpicList, error) {
	var l wire.EpicList
	err := c.get(ctx, "/v1/epics", &l)
	return l, err
}

// EpicDetail returns an epic's state, progress and ticket rows.
func (c *Client) EpicDetail(ctx context.Context, address string) (wire.EpicDetail, error) {
	var d wire.EpicDetail
	err := c.get(ctx, "/v1/epics/detail?address="+url.QueryEscape(address), &d)
	return d, err
}

// Stats returns the per-day stats for one range in days (0 is the server's
// default) and one project (empty means all).
func (c *Client) Stats(ctx context.Context, days int, project string) (wire.Stats, error) {
	q := url.Values{}
	if days > 0 {
		q.Set("range", strconv.Itoa(days))
	}
	if project != "" {
		q.Set("project", project)
	}
	var s wire.Stats
	err := c.get(ctx, "/v1/stats?"+q.Encode(), &s)
	return s, err
}

// Iterations returns the live iterations.
func (c *Client) Iterations(ctx context.Context) ([]wire.IterationInfo, error) {
	var l []wire.IterationInfo
	err := c.get(ctx, "/v1/iterations", &l)
	return l, err
}

// Queue returns every ticket with the scheduler's verdict on it.
func (c *Client) Queue(ctx context.Context) ([]wire.QueueEntry, error) {
	var l []wire.QueueEntry
	err := c.get(ctx, "/v1/queue", &l)
	return l, err
}

// QueueItems returns the server-wide queue in order.
func (c *Client) QueueItems(ctx context.Context) ([]wire.QueueItem, error) {
	var l []wire.QueueItem
	err := c.get(ctx, "/v1/queue/items", &l)
	return l, err
}
