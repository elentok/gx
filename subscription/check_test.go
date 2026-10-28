package subscription

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFreshReadsHomeClaudeJSON(t *testing.T) {
	home := t.TempDir()
	if got := CheckFresh(home); got != StateUnknown {
		t.Fatalf("CheckFresh(no file) = %v, want StateUnknown", got)
	}
	data := []byte(`{"oauthAccount":{"hasExtraUsageEnabled":true}}`)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CheckFresh(home); got != StateEnabled {
		t.Fatalf("CheckFresh() = %v, want StateEnabled", got)
	}
}

func TestDetectEnabled(t *testing.T) {
	data := []byte(`{"oauthAccount":{"hasExtraUsageEnabled":true}}`)
	if got := Detect(data); got != StateEnabled {
		t.Fatalf("Detect() = %v, want StateEnabled", got)
	}
}

func TestDetectDisabled(t *testing.T) {
	data := []byte(`{"oauthAccount":{"hasExtraUsageEnabled":false}}`)
	if got := Detect(data); got != StateDisabled {
		t.Fatalf("Detect() = %v, want StateDisabled", got)
	}
}

func TestDetectMissingFieldIsUnknown(t *testing.T) {
	data := []byte(`{"oauthAccount":{"emailAddress":"a@b.com"}}`)
	if got := Detect(data); got != StateUnknown {
		t.Fatalf("Detect() = %v, want StateUnknown", got)
	}
}

func TestDetectMissingOauthAccountIsUnknown(t *testing.T) {
	data := []byte(`{}`)
	if got := Detect(data); got != StateUnknown {
		t.Fatalf("Detect() = %v, want StateUnknown", got)
	}
}

func TestDetectRenamedFieldIsUnknown(t *testing.T) {
	data := []byte(`{"oauthAccount":{"hasExtraUsageEnabledRenamed":true}}`)
	if got := Detect(data); got != StateUnknown {
		t.Fatalf("Detect() = %v, want StateUnknown", got)
	}
}

func TestDetectUnexpectedShapeIsUnknown(t *testing.T) {
	data := []byte(`{"oauthAccount":{"hasExtraUsageEnabled":"not-a-bool"}}`)
	if got := Detect(data); got != StateUnknown {
		t.Fatalf("Detect() = %v, want StateUnknown", got)
	}
}

func TestDetectMalformedJSONIsUnknown(t *testing.T) {
	data := []byte(`not json`)
	if got := Detect(data); got != StateUnknown {
		t.Fatalf("Detect() = %v, want StateUnknown", got)
	}
}
