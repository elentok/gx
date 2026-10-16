package agents

import (
	"context"
	"sync"

	"github.com/elentok/gx/server"
)

// fakeSessionClient records every call and answers with result.
type fakeSessionClient struct {
	mu     sync.Mutex
	calls  []string
	result server.SessionResult
	err    error
}

var _ SessionClient = (*fakeSessionClient)(nil)

func (f *fakeSessionClient) record(call string) (server.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.result, f.err
}

func (f *fakeSessionClient) CreateSession(_ context.Context, req server.CreateSessionRequest) (server.SessionResult, error) {
	return f.record("create " + req.Project)
}

func (f *fakeSessionClient) StopSession(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("stop " + id)
}

func (f *fakeSessionClient) RemoveSession(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("remove " + id)
}
