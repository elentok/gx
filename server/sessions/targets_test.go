package sessions_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

func TestSessionTargetsWrite_InternalErrorOnLaterTargetKeepsEarlierResults(t *testing.T) {
	do := func(id string, _ bool) (wire.SessionResult, error) {
		if id == "b" {
			return wire.SessionResult{}, errors.New("disk full")
		}
		return wire.SessionResult{}, nil
	}
	body := `{"targets":[{"id":"a"},{"id":"b"},{"id":"c"}]}`
	w := httptest.NewRecorder()
	new(sessions.Manager).SessionTargetsWrite(do, false)(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-target results", w.Code)
	}
	var out wire.TargetsResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("results = %+v, want one per target", out.Results)
	}
	if out.Results[0].Status != wire.TargetDone || out.Results[2].Status != wire.TargetDone {
		t.Errorf("results = %+v, want a and c done", out.Results)
	}
	if got := out.Results[1]; got.Target != "b" || got.Status != wire.TargetFailed || !strings.Contains(got.Message, "disk full") {
		t.Errorf("b = %+v, want failed with the error", got)
	}
}
