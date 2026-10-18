package terminalrun

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProbeGhostty(t *testing.T) {
	calls := fakeOsa(t, "", nil)
	if err := ProbeGhostty(); err != nil {
		t.Fatal(err)
	}
	want := []osaCall{{"osascript", []string{"-e", `tell application "Ghostty" to count windows`}}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %#v\nwant %#v", *calls, want)
	}

	fakeOsa(t, "execution error: Not authorized (-1743)", errors.New("exit 1"))
	var fb *FallbackError
	err := ProbeGhostty()
	if !errors.As(err, &fb) || !strings.Contains(fb.Hint, "Automation") {
		t.Fatalf("err = %v, want Automation hint", err)
	}
}
