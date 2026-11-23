package deliver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/agentrunner/deliver"
)

func codexSlash(text string) deliver.Policy {
	p := deliver.PolicyFor(text)
	p.Slash.Settle = 0
	p.Slash.Marker = codexui.UnknownCommandMarker
	p.Slash.Watch = 4 * time.Millisecond
	p.Slash.WatchPoll = time.Millisecond
	return p
}

func TestDeliver_CodexSlashTypesOnceAndPressesOneEnter(t *testing.T) {
	f := &slashFake{reads: []read{ok([]string{"› /model"})}}
	if err := deliver.Deliver(f, "/model", codexSlash("/model")); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if f.enters != 1 || typedCount(f) != 1 {
		t.Fatalf("enters %d, typed %d, want one each (calls %v)", f.enters, typedCount(f), f.calls)
	}
	if strings.Contains(strings.Join(f.calls, " "), "submit:") {
		t.Fatalf("calls = %v: a slash send does not use Submit", f.calls)
	}
}

func TestDeliver_CodexIgnoresAnUnrecognizedCommandLeftByAnEarlierSend(t *testing.T) {
	stale := []string{"■ " + codexui.UnknownCommandMarker + " `/nope`", "› /model"}
	f := &slashFake{reads: []read{ok(stale)}}
	if err := deliver.Deliver(f, "/model", codexSlash("/model")); err != nil {
		t.Fatalf("Deliver() error = %v, want nil: the marker was on the pane before the send", err)
	}
}

func TestDeliver_CodexUnrecognizedCommandFailsTheSend(t *testing.T) {
	f := &slashFake{reads: []read{ok([]string{"› /nope"}), ok([]string{"■ " + codexui.UnknownCommandMarker + " `/nope`"})}}
	err := deliver.Deliver(f, "/nope", codexSlash("/nope"))
	if !errors.Is(err, agentrunner.ErrNotDelivered) || !errors.Is(err, agentrunner.ErrUnknownSkill) {
		t.Fatalf("Deliver() error = %v, want ErrNotDelivered and ErrUnknownSkill", err)
	}
	if f.enters != 1 {
		t.Fatalf("enters = %d, want 1: no retype and no settle Enter", f.enters)
	}
}
