package codexsetup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, o Options) (string, error) {
	t.Helper()
	var out bytes.Buffer
	o.Out = &out
	if o.In == nil {
		o.In = strings.NewReader("")
	}
	err := Run(o)
	return out.String(), err
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRun_FreshDirWritesRulesFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	p := RulesPath(home)
	if !strings.HasPrefix(read(t, p), Marker+"\n") {
		t.Errorf("rules file lacks marker")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	if !strings.Contains(out, "does not exist; skipped gopls approvals") {
		t.Errorf("want skip message for missing config.toml, got %q", out)
	}
	if !strings.Contains(out, "Restart Codex for the changes to take effect.") {
		t.Errorf("missing restart line: %q", out)
	}
}

func TestRun_SecondRunChangesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, filepath.Join(home, "config.toml"), "[mcp_servers.gopls-mcp]\ncommand = \"gopls\"\n")
	if _, err := run(t, Options{CodexHome: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	rules, cfg := RulesPath(home), filepath.Join(home, "config.toml")
	beforeR, beforeC := read(t, rules), read(t, cfg)
	fiR, _ := os.Stat(rules)
	fiC, _ := os.Stat(cfg)
	time.Sleep(20 * time.Millisecond)

	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already set up") {
		t.Errorf("out = %q, want already set up", out)
	}
	if read(t, rules) != beforeR || read(t, cfg) != beforeC {
		t.Errorf("bytes changed on second run")
	}
	fiR2, _ := os.Stat(rules)
	fiC2, _ := os.Stat(cfg)
	if !fiR.ModTime().Equal(fiR2.ModTime()) || !fiC.ModTime().Equal(fiC2.ModTime()) {
		t.Errorf("mtime changed on second run")
	}
}

func TestRun_UnmanagedRulesFileUntouched(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, RulesPath(home), "# mine\n")
	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, RulesPath(home)) != "# mine\n" {
		t.Errorf("unmanaged file was changed")
	}
	if !strings.Contains(out, "not managed by gx") {
		t.Errorf("out = %q, want unmanaged note", out)
	}
}

func TestRun_OldMarkerUpdatedInPlace(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, RulesPath(home), OldMarker+"\nold body\n")
	if _, err := run(t, Options{CodexHome: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, RulesPath(home)); got != RulesContent() {
		t.Errorf("rules not updated:\n%s", got)
	}
}

func TestApproveGopls_KeepsOtherContent(t *testing.T) {
	t.Parallel()
	in := `# top comment
model = "x"

[mcp_servers.other]
enabled_tools = ["a"]

[mcp_servers.gopls-mcp]
command = "gopls" # keep
enabled = false
enabled_tools = [
  "a",
  "b",
]
disabled_tools = ["c"]

[tui]
theme = "dark"
`
	want := `# top comment
model = "x"

[mcp_servers.other]
enabled_tools = ["a"]

[mcp_servers.gopls-mcp]
command = "gopls" # keep
enabled = true
default_tools_approval_mode = "approve"

[tui]
theme = "dark"
`
	if got := approveGopls(in, "gopls-mcp"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRun_GoplsFallbackAndNamedServer(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	write(t, cfg, "[mcp_servers.gopls]\ncommand = \"g\"\n\n[mcp_servers.custom]\ncommand = \"c\"\n")
	if _, err := run(t, Options{CodexHome: home, Yes: true, GoplsServer: "custom"}); err != nil {
		t.Fatal(err)
	}
	want := "[mcp_servers.gopls]\ncommand = \"g\"\n\n[mcp_servers.custom]\ncommand = \"c\"\nenabled = true\ndefault_tools_approval_mode = \"approve\"\n"
	if got := read(t, cfg); got != want {
		t.Errorf("got:\n%s", got)
	}
}

func TestRun_NoGoplsServerSkips(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	write(t, cfg, "[mcp_servers.other]\ncommand = \"c\"\n")
	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, cfg) != "[mcp_servers.other]\ncommand = \"c\"\n" {
		t.Errorf("config changed")
	}
	if !strings.Contains(out, "no gopls-mcp or gopls MCP server is configured") {
		t.Errorf("out = %q", out)
	}
}

func TestRun_PreviewNamesPathsBeforeWrite(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, filepath.Join(home, "config.toml"), "[mcp_servers.gopls]\ncommand = \"g\"\n")
	out, err := run(t, Options{CodexHome: home, Interactive: true, In: strings.NewReader("n\n")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{RulesPath(home), filepath.Join(home, "config.toml"), "+enabled = true", "Write these changes? [y/N]"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(RulesPath(home)); err == nil {
		t.Errorf("rules file written after answering n")
	}
	if strings.Contains(read(t, filepath.Join(home, "config.toml")), "enabled") {
		t.Errorf("config written after answering n")
	}
}

func TestRun_YesAnswerWrites(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := run(t, Options{CodexHome: home, Interactive: true, In: strings.NewReader("y\n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(RulesPath(home)); err != nil {
		t.Errorf("rules file not written: %v", err)
	}
}

func TestRun_NoTTYWithoutYesRefuses(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := run(t, Options{CodexHome: home}); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(RulesPath(home)); err == nil {
		t.Errorf("rules file written without tty or --yes")
	}
}
