package subscription

import "testing"

func TestBuildLineEnabledWarns(t *testing.T) {
	line := BuildLine(StateEnabled, false)
	if line == nil || line.Severity != SeverityWarning {
		t.Fatalf("BuildLine(Enabled, false) = %+v, want a warning line", line)
	}
}

func TestBuildLineEnabledSuppressed(t *testing.T) {
	if line := BuildLine(StateEnabled, true); line != nil {
		t.Fatalf("BuildLine(Enabled, true) = %+v, want nil", line)
	}
}

func TestBuildLineDisabledIsQuiet(t *testing.T) {
	line := BuildLine(StateDisabled, false)
	if line == nil || line.Severity != SeverityInfo {
		t.Fatalf("BuildLine(Disabled, false) = %+v, want an info line", line)
	}
}

func TestBuildLineUnknownIsQuiet(t *testing.T) {
	line := BuildLine(StateUnknown, false)
	if line == nil || line.Severity != SeverityInfo {
		t.Fatalf("BuildLine(Unknown, false) = %+v, want an info line", line)
	}
}

func TestBuildLineDisabledIgnoresSuppress(t *testing.T) {
	line := BuildLine(StateDisabled, true)
	if line == nil {
		t.Fatal("BuildLine(Disabled, true) = nil, want a line (suppress only affects the enabled warning)")
	}
}
