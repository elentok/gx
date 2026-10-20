package runnerfake_test

import (
	"errors"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestAnswer_RequestIDMustMatchPending(t *testing.T) {
	r := runnerfake.NewRunner()
	s, err := r.Start(agentrunner.StartOptions{Label: "e-01"})
	if err != nil {
		t.Fatal(err)
	}
	r.SetPending(s.Label, &agentrunner.Pending{ID: "req-1", Tool: "Bash"})

	if err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-2"}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("Answer with a wrong id = %v, want ErrRequestMismatch", err)
	}
	if err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-1"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.Status(s); st.Pending != nil {
		t.Errorf("Pending after Answer = %+v, want nil", st.Pending)
	}
}
