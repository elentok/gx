package agents

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui"
)

// fakeSessionClient records every call and answers with result.
type fakeSessionClient struct {
	mu     sync.Mutex
	calls  []string
	result wire.SessionResult
	err    error

	creates []wire.CreateSessionRequest

	snapshot      wire.Snapshot
	queueRefusals map[string]string

	reports     []wire.FocusReport
	focusResult wire.SessionResult
}

// One recorder plays both the client and its Sessions sub-client, so a test
// reads every call from one calls list.
var (
	_ SessionClient = (*fakeSessionClient)(nil)
	_ Sessions      = (*fakeSessionClient)(nil)
)

func newTestModel(f *fakeSessionClient, settings ui.Settings) Model {
	return NewModel(f, f, settings)
}

func (f *fakeSessionClient) record(call string) (wire.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.result, f.err
}

func (f *fakeSessionClient) CreateSession(_ context.Context, req wire.CreateSessionRequest) (wire.SessionResult, error) {
	f.mu.Lock()
	f.creates = append(f.creates, req)
	f.mu.Unlock()
	return f.record("create " + req.Project)
}

func (f *fakeSessionClient) Stop(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("stop " + id)
}

func (f *fakeSessionClient) Remove(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("remove " + id)
}

func (f *fakeSessionClient) Resume(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("resume " + id)
}

func (f *fakeSessionClient) MarkRead(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("mark-read " + id)
}

func (f *fakeSessionClient) RunAction(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("action " + id)
}

func (f *fakeSessionClient) SetAutoAdvance(_ context.Context, id string, on bool) (wire.SessionResult, error) {
	return f.record("auto-advance " + id + " " + strconv.FormatBool(on))
}

func (f *fakeSessionClient) CancelAutoAdvance(_ context.Context, id string) (wire.SessionResult, error) {
	return f.record("cancel-auto-advance " + id)
}

func (f *fakeSessionClient) Snapshot(context.Context) (wire.Snapshot, error) {
	return f.snapshot, nil
}

// QueueAdd refuses the addresses in queueRefusals with the reason they map to.
func (f *fakeSessionClient) QueueAdd(_ context.Context, address, _ string) (wire.QueueResult, error) {
	f.record("queue " + address)
	if reason, ok := f.queueRefusals[address]; ok {
		return wire.QueueResult{Refused: true, Message: reason}, nil
	}
	return wire.QueueResult{}, nil
}

func (f *fakeSessionClient) SetAFK(_ context.Context, on *bool) (wire.AFKResult, error) {
	call := "afk toggle"
	if on != nil {
		call = "afk " + strconv.FormatBool(*on)
	}
	_, err := f.record(call)
	return wire.AFKResult{}, err
}

// ReportFocus records reports apart from calls: every update may send one.
func (f *fakeSessionClient) ReportFocus(_ context.Context, req wire.FocusReport) (wire.SessionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, req)
	return f.focusResult, nil
}

// waitReport waits for the reporter to send want.
func (f *fakeSessionClient) waitReport(t *testing.T, want wire.FocusReport) {
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
