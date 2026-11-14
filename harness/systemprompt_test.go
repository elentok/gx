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
	want := []string{"--x", "--append-system-prompt", harness.GXLoopPrompt}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestWithGXSystemPrompt_SkipsOnEitherFlag(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--append-system-prompt", "--append-system-prompt-file"} {
		in := []string{flag, "v"}
		if got := harness.WithGXSystemPrompt(in); !slices.Equal(got, in) {
			t.Errorf("with %s: args = %q, want unchanged", flag, got)
		}
	}
}

func TestGXFilesSentence_IsTheSpecTextOnOneLine(t *testing.T) {
	t.Parallel()
	want := "To show the person a file you made (Markdown, HTML, a Mermaid diagram, a mock), never start your own HTTP server. Run `gx files url <path>` and send that link. gx hosts the file behind its web login. If the command refuses the path, save the file in the repo or a temp folder and run it again."
	if harness.GXFilesSentence != want {
		t.Errorf("GXFilesSentence = %q", harness.GXFilesSentence)
	}
	if strings.ContainsAny(harness.GXFilesSentence, "\r\n") {
		t.Error("GXFilesSentence contains a newline")
	}
}

func TestGXLoopPrompt_AskThenFilesOnOneLine(t *testing.T) {
	t.Parallel()
	if harness.GXLoopPrompt != harness.GXSystemPrompt+" "+harness.GXFilesSentence {
		t.Error("GXLoopPrompt is not the ask text then the files sentence")
	}
	if strings.ContainsAny(harness.GXLoopPrompt, "\r\n") {
		t.Error("GXLoopPrompt contains a newline")
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

func TestGXSystemPromptFor_Codex(t *testing.T) {
	t.Parallel()
	got := harness.GXSystemPromptFor(harness.Codex)
	if strings.Contains(got, "AskUserQuestion") || !strings.Contains(got, "request_user_input") {
		t.Errorf("codex prompt names the wrong question tool: %q", got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("codex prompt contains a newline: %q", got)
	}
	if want := strings.Replace(harness.GXSystemPrompt, "AskUserQuestion", "request_user_input", 1) + " " + harness.GXFilesSentence; got != want {
		t.Errorf("codex prompt is not the loop prompt with the tool renamed in the ask part")
	}
	if harness.GXSystemPromptFor(harness.Claude) != harness.GXLoopPrompt {
		t.Error("claude prompt changed")
	}
}

func TestTOMLString(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`plain`:        `"plain"`,
		`say "hi"`:     `"say \"hi\""`,
		`a\b`:          `"a\\b"`,
		"two\nlines\t": `"two\nlines\t"`,
		"bell\x07":     `"bell\u0007"`,
		"héllo → ok":   `"héllo → ok"`,
	} {
		if got := harness.TOMLString(in); got != want {
			t.Errorf("TOMLString(%q) = %s, want %s", in, got, want)
		}
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
	if string(got) != harness.GXLoopPrompt {
		t.Errorf("file = %q, want the prompt", got)
	}
	if again := harness.GXSystemPromptFile(); again != path {
		t.Errorf("second call = %q, want %q", again, path)
	}
}
