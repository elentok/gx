package cmd

import (
	"context"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
)

// tuiClient lets *apiclient.Client satisfy the tab interfaces until they
// take the sub-clients directly; the follow-up that shrinks ServerAPI,
// SessionClient and ServerClient deletes it.
type tuiClient struct{ *apiclient.Client }

func (c tuiClient) StopSession(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.Stop(ctx, id)
}

func (c tuiClient) RemoveSession(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.Remove(ctx, id)
}

func (c tuiClient) ResumeSession(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.Resume(ctx, id)
}

func (c tuiClient) MarkSessionRead(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.MarkRead(ctx, id)
}

func (c tuiClient) RunSessionAction(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.RunAction(ctx, id)
}

func (c tuiClient) SetAutoAdvance(ctx context.Context, id string, on bool) (wire.SessionResult, error) {
	return c.Sessions.SetAutoAdvance(ctx, id, on)
}

func (c tuiClient) CancelAutoAdvance(ctx context.Context, id string) (wire.SessionResult, error) {
	return c.Sessions.CancelAutoAdvance(ctx, id)
}

func (c tuiClient) ReportFocus(ctx context.Context, req wire.FocusReport) (wire.SessionResult, error) {
	return c.Sessions.ReportFocus(ctx, req)
}

func (c tuiClient) SetAFK(ctx context.Context, on *bool) (wire.AFKResult, error) {
	return c.Sessions.SetAFK(ctx, on)
}

func (c tuiClient) DesignWorktree(ctx context.Context, req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error) {
	return c.Sessions.DesignWorktree(ctx, req)
}

func (c tuiClient) FixAgent(ctx context.Context, req wire.FixAgentRequest) (wire.FixAgentResult, error) {
	return c.Sessions.FixAgent(ctx, req)
}
