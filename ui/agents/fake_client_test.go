package agents

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// fakeSessionClient records every call and answers with result.
type fakeSessionClient struct {
	mu     sync.Mutex
	calls  []string
	result server.SessionResult
	err    error

	creates []server.CreateSessionRequest

	reports     []server.FocusReport
	focusResult server.SessionResult
}

var _ SessionClient = (*fakeSessionClient)(nil)

func (f *fakeSessionClient) record(call string) (server.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.result, f.err
}

func (f *fakeSessionClient) CreateSession(_ context.Context, req server.CreateSessionRequest) (server.SessionResult, error) {
	f.mu.Lock()
	f.creates = append(f.creates, req)
	f.mu.Unlock()
	return f.record("create " + req.Project)
}

func (f *fakeSessionClient) StopSession(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("stop " + id)
}

func (f *fakeSessionClient) RemoveSession(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("remove " + id)
}

func (f *fakeSessionClient) ResumeSession(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("resume " + id)
}

// ReportFocus records reports apart from calls: every update may send one.
func (f *fakeSessionClient) ReportFocus(_ context.Context, req server.FocusReport) (server.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, req)
	return f.focusResult, nil
}

// waitReport waits for the reporter to send want.
func (f *fakeSessionClient) waitReport(t *testing.T, want server.FocusReport) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		reports := slices.Clone(f.reports)
		f.mu.Unlock()
		if len(reports) > 0 && reports[len(reports)-1] == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("focus reports = %+v, want the last to be %+v", reports, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
