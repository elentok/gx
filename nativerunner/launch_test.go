package nativerunner

import (
	"slices"
	"testing"
)

func TestLaunchEnv_StripsNestedClaudeVarsAndDisablesAutoupdater(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"CLAUDECODE=1",
		"CLAUDE_CODE_ENTRYPOINT=cli",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"CLAUDE_CONFIG_DIR=/cfg",
		"CLAUDECODE_EXTRA=keep",
	}
	orig := slices.Clone(base)
	got := LaunchEnv(base, []string{"FOO=bar"})
	want := []string{
		"PATH=/bin",
		"CLAUDE_CONFIG_DIR=/cfg",
		"CLAUDECODE_EXTRA=keep",
		"DISABLE_AUTOUPDATER=1",
		"FOO=bar",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("LaunchEnv = %q, want %q", got, want)
	}
	if !slices.Equal(base, orig) {
		t.Fatalf("LaunchEnv modified base: %q", base)
	}
}
