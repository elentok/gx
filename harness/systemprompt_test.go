package harness_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

func TestWithGXSystemPrompt_AppendsOnce(t *testing.T) {
	got := harness.WithGXSystemPrompt(harness.WithGXSystemPrompt([]string{"--x"}))
	want := []string{"--x", "--append-system-prompt", harness.GXSystemPrompt}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

// Agents-tab and gx-design sessions go through Launch and Resume as is: the
// system prompt is the loop path's to add.
func TestClaudeLaunchAndResume_OmitGXSystemPrompt(t *testing.T) {
	h, err := harness.Lookup(harness.Claude)
	if err != nil {
		t.Fatal(err)
	}
	launch := h.Launch(harness.LaunchOptions{SessionID: "s"})
	resume, _ := h.Resume("s", harness.LaunchOptions{})
	for name, cmd := range map[string]harness.Command{"launch": launch, "resume": resume} {
		if slices.Contains(cmd.Args, "--append-system-prompt") {
			t.Errorf("%s args %q carry --append-system-prompt", name, cmd.Args)
		}
	}
}

// herdr refuses a launch argument with a newline (invalid_agent_argument), so
// the prompt must stay on one line.
func TestGXSystemPrompt_IsOneLine(t *testing.T) {
	if strings.ContainsAny(harness.GXSystemPrompt, "\r\n") {
		t.Errorf("GXSystemPrompt contains a newline: %q", harness.GXSystemPrompt)
	}
}

func TestGXSystemPromptFile_HoldsThePrompt(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	path := harness.GXSystemPromptFile()
	if path == "" {
		t.Fatal("no file written")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != harness.GXSystemPrompt {
		t.Errorf("file = %q, want the prompt", got)
	}
	if again := harness.GXSystemPromptFile(); again != path {
		t.Errorf("second call = %q, want %q", again, path)
	}
}
