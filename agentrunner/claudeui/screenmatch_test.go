package claudeui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/screen"
)

// The fixtures are claude 2.1.296 screens recorded by the phase 2 map
// (evidence under gx:agent-runner-phase2): *.screen.txt are screen dumps and
// trust-w60.raw is raw PTY output, replayed so an x/vt pin bump that breaks
// the anchors fails here.

func screenFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".screen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return ScreenText(nil, strings.Split(string(b), "\n"))
}

func replayFixture(t *testing.T, name string, cols, rows int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".raw"))
	if err != nil {
		t.Fatal(err)
	}
	s := screen.NewScreen(cols, rows)
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Write(raw); err != nil {
		t.Fatal(err)
	}
	return ScreenText(s.ScrollbackTail(100), s.Rows())
}

func fixtureRows(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".screen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func TestInputBox(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		want bool
	}{
		{"startup screen with its input box", fixtureRows(t, "startup-w250"), true},
		{"trust dialog", fixtureRows(t, "trust-w160"), false},
		{"permission prompt", fixtureRows(t, "permission-w160"), false},
		{"plan approval", fixtureRows(t, "plan-approval"), false},
		{"blank screen", []string{"", ""}, false},
	}
	for _, c := range cases {
		if got := InputBox(c.rows); got != c.want {
			t.Errorf("%s: InputBox = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassifyStartup(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Startup
	}{
		{"trust dialog at 60 cols, hard-wrapped", replayFixture(t, "trust-w60", 60, 30), StartupTrust},
		{"trust dialog at 160 cols", screenFixture(t, "trust-w160"), StartupTrust},
		{"startup banner, no pid file yet", screenFixture(t, "startup-w250"), StartupNoSession},
		{"blank screen", ScreenText(nil, []string{"", ""}), StartupNoSession},
		{"prompt quoting one anchor", Normalize("❯ reply with: Yes, I trust this folder"), StartupNoSession},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyStartup(c.text); got != c.want {
				t.Errorf("ClassifyStartup = %v, want %v\ntext: %q", got, c.want, c.text)
			}
		})
	}
}

func TestTrustKey(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "trust-w160.screen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	recorded := string(b)
	onYes := strings.NewReplacer("❯ No, exit", "  No, exit", "  Yes, I trust", "❯ Yes, I trust").Replace(recorded)
	if onYes == recorded {
		t.Fatal("fixture no longer has the cursor on No, exit")
	}
	cases := []struct {
		name string
		rows string
		want string
	}{
		{"recorded dialog, cursor on No, exit", recorded, "\x1b[B"},
		{"cursor moved to the trust option", onYes, "\r"},
		{"no cursor drawn yet", "Quick safety check", "\x1b[B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TrustKey(strings.Split(c.rows, "\n")); got != c.want {
				t.Errorf("TrustKey = %q, want %q", got, c.want)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	const proceed = "Do you want to proceed?"
	cases := []struct {
		name   string
		text   string
		anchor string
		want   bool
	}{
		{"recorded permission prompt between dashed borders", screenFixture(t, "permission-w160"), proceed, true},
		{
			"side-bordered dialog wrapped mid-anchor",
			ScreenText(nil, []string{"╭──────────────╮", "│ Do you want │", "│  to proceed? │", "╰──────────────╯"}),
			proceed, true,
		},
		{"gutter before a wrapped reply", ScreenText([]string{"⏺ Lighthouses have"}, []string{"  guided sailors"}), "Lighthouses have guided sailors", true},
		{"anchor whitespace is normalized too", Normalize("a  b"), "a\n b", true},
		{"absent anchor", screenFixture(t, "trust-w160"), proceed, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Matches(c.text, c.anchor); got != c.want {
				t.Errorf("Matches(%q) = %v, want %v\ntext: %q", c.anchor, got, c.want, c.text)
			}
		})
	}
}

// The pty runner's allow is Enter on the selected first option, so it must be
// a "Yes" on every dialog claude reports as a permission prompt.
func TestPermissionPromptsSelectYesFirst(t *testing.T) {
	firstOption := regexp.MustCompile(`❯ 1\. (\S+)`)
	for _, name := range []string{"permission-w160", "plan-approval"} {
		t.Run(name, func(t *testing.T) {
			m := firstOption.FindStringSubmatch(screenFixture(t, name))
			if m == nil || !strings.HasPrefix(m[1], "Yes") {
				t.Fatalf("selected first option = %q, want Yes", m)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	got := Normalize(" Quick safety check: Is this a project you created or one", " you trust?\t(Like", "│ ⏺ x │")
	want := "Quick safety check: Is this a project you created or one you trust? (Like x"
	if got != want {
		t.Errorf("Normalize = %q, want %q", got, want)
	}
}
