package agents

import (
	"context"
	"slices"
	"strconv"
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

	snapshot      server.Snapshot
	queueRefusals map[string]string

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

func (f *fakeSessionClient) MarkSessionRead(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("mark-read " + id)
}

func (f *fakeSessionClient) RunSessionAction(_ context.Context, id string) (server.SessionResult, error) {
	return f.record("action " + id)
}

func (f *fakeSessionClient) Snapshot(context.Context) (server.Snapshot, error) {
	return f.snapshot, nil
}

// QueueAdd refuses the addresses in queueRefusals with the reason they map to.
func (f *fakeSessionClient) QueueAdd(_ context.Context, address, _ string) (server.QueueResult, error) {
	f.record("queue " + address)
	if reason, ok := f.queueRefusals[address]; ok {
		return server.QueueResult{Refused: true, Message: reason}, nil
	}
	return server.QueueResult{}, nil
}

func (f *fakeSessionClient) SetAFK(_ context.Context, on *bool) (server.AFKResult, error) {
	call := "afk toggle"
	if on != nil {
		call = "afk " + strconv.FormatBool(*on)
	}
	_, err := f.record(call)
	return server.AFKResult{}, err
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
