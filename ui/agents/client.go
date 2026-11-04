package agents

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// SessionClient is the slice of apiclient.Client the Agents tab uses to act on
// the server's interactive sessions.
type SessionClient interface {
	CreateSession(ctx context.Context, req wire.CreateSessionRequest) (wire.SessionResult, error)
	StopSession(ctx context.Context, id string) (wire.SessionResult, error)
	RemoveSession(ctx context.Context, id string) (wire.SessionResult, error)
	ResumeSession(ctx context.Context, id string) (wire.SessionResult, error)
	MarkSessionRead(ctx context.Context, id string) (wire.SessionResult, error)
	RunSessionAction(ctx context.Context, id string) (wire.SessionResult, error)
	SetAutoAdvance(ctx context.Context, id string, on bool) (wire.SessionResult, error)
	CancelAutoAdvance(ctx context.Context, id string) (wire.SessionResult, error)
	Snapshot(ctx context.Context) (wire.Snapshot, error)
	QueueAdd(ctx context.Context, address, agent string) (wire.QueueResult, error)
	ReportFocus(ctx context.Context, req wire.FocusReport) (wire.SessionResult, error)
	SetAFK(ctx context.Context, on *bool) (wire.AFKResult, error)
}
