package agents

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// SessionClient is the slice of apiclient.Client the Agents tab still reads
// from the client itself; the session verbs live on Sessions.
type SessionClient interface {
	Snapshot(ctx context.Context) (wire.Snapshot, error)
	QueueAdd(ctx context.Context, address, agent string) (wire.QueueResult, error)
}

// Sessions is the slice of *apiclient.Sessions the Agents tab uses to act on
// the server's interactive sessions.
type Sessions interface {
	Create(ctx context.Context, req wire.CreateSessionRequest) (wire.SessionResult, error)
	Stop(ctx context.Context, id string) (wire.SessionResult, error)
	Remove(ctx context.Context, id string) (wire.SessionResult, error)
	Resume(ctx context.Context, id string) (wire.SessionResult, error)
	MarkRead(ctx context.Context, id string) (wire.SessionResult, error)
	RunAction(ctx context.Context, id string) (wire.SessionResult, error)
	SetAutoAdvance(ctx context.Context, id string, on bool) (wire.SessionResult, error)
	CancelAutoAdvance(ctx context.Context, id string) (wire.SessionResult, error)
	ReportFocus(ctx context.Context, req wire.FocusReport) (wire.SessionResult, error)
	SetAFK(ctx context.Context, on *bool) (wire.AFKResult, error)
}
