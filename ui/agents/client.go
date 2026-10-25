package agents

import (
	"context"

	"github.com/elentok/gx/server"
)

// SessionClient is the slice of apiclient.Client the Agents tab uses to act on
// the server's interactive sessions.
type SessionClient interface {
	CreateSession(ctx context.Context, req server.CreateSessionRequest) (server.SessionResult, error)
	StopSession(ctx context.Context, id string) (server.SessionResult, error)
	RemoveSession(ctx context.Context, id string) (server.SessionResult, error)
	ResumeSession(ctx context.Context, id string) (server.SessionResult, error)
	MarkSessionRead(ctx context.Context, id string) (server.SessionResult, error)
	RunSessionAction(ctx context.Context, id string) (server.SessionResult, error)
	SetAutoAdvance(ctx context.Context, id string, on bool) (server.SessionResult, error)
	CancelAutoAdvance(ctx context.Context, id string) (server.SessionResult, error)
	Snapshot(ctx context.Context) (server.Snapshot, error)
	QueueAdd(ctx context.Context, address, agent string) (server.QueueResult, error)
	ReportFocus(ctx context.Context, req server.FocusReport) (server.SessionResult, error)
	SetAFK(ctx context.Context, on *bool) (server.AFKResult, error)
}
