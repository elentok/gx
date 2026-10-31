package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestGuardBashCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		command string
		blocked bool
	}{
		{"sed -i", `sed -i 's/a/b/' queue.go`, true},
		{"sed -i", `sed -i 's/a/b/' queue.go`, true},
		{"sed -i -E", `sed -i -E 's/a/b/' x_test.go`, true},
		{"sed -Ei", `sed -Ei 's/a/b/' x_test.go`, true},
		{"sed --in-place", `sed --in-place 's/a/b/' x.go`, true},
		{"cat append heredoc", "cat >> a_test.go <<'EOF'\nfunc T() {}\nEOF", true},
		{"cat overwrite heredoc", "cat > a.go <<EOF\npackage a\nEOF", true},
		{"python heredoc", "python3 - <<'PY'\nprint(1)\nPY", true},
		{"sed print", `sed -n '1,5p' queue.go`, false},
		{"sed pipe", `echo a | sed 's/a/b/'`, false},
		{"plain redirect", `echo hi > out.txt`, false},
		{"heredoc to stdin", "gx tickets section 1/2 Notes - <<'EOF'\nbody\nEOF", false},
		{"heredoc with stderr merge", "git commit -F - <<'EOF' 2>&1\nmsg\nEOF", false},
		{"heredoc with dev null", "git commit -F - <<'EOF' >/dev/null\nmsg\nEOF", false},
		{"ticket add body with arrows", "gx tickets add e --slug s --body - <<'EOF'\nBlock `>` and a -> b, not >>x\nEOF", false},
		{"redirect after heredoc body", "gx tickets add e --body - <<'EOF'\nbody\nEOF\necho hi > out.txt", true},
		{"heredoc start with redirect", "cat <<'EOF' > a.go\npackage a\nEOF", true},
		{"commit trailer ending in >\"", "gx tickets section 1/2 Notes - <<'EOF'\nbody\nEOF\ngit commit -m \"subj\n\nCo-Authored-By: Claude <noreply@anthropic.com>\"", false},
		{"redirect into quoted path", "cat > \"a.go\" <<'EOF'\npackage a\nEOF", true},
		{"redirect after unbalanced apostrophe", "echo it's > a.go <<EOF\nx\nEOF", true},
		{"redirect between stray and real quotes", "echo it's > a.go; echo 'x' <<EOF\nx\nEOF", true},
		{"git status", `git status`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := guardBashCommand(tc.command)
			if (got != "") != tc.blocked {
				t.Errorf("guardBashCommand(%q) = %q, blocked want %v", tc.command, got, tc.blocked)
			}
			if tc.blocked && !strings.Contains(got, "Edit or Write") {
				t.Errorf("reason %q does not name Edit/Write", got)
			}
		})
	}
}

func TestAgentGuardBash_CodexHarness(t *testing.T) {
	t.Parallel()
	run := func(stdin string) (string, error) {
		c := newAgentGuardBashCmd()
		var stderr bytes.Buffer
		c.SetArgs([]string{"--harness", "codex"})
		c.SetIn(strings.NewReader(stdin))
		c.SetErr(&stderr)
		err := c.Execute()
		return stderr.String(), err
	}
	stderr, err := run(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"sed -i s/a/b/ x.go"}}`)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != guardBashExitCode {
		t.Fatalf("err = %v, want ExitError{%d}", err, guardBashExitCode)
	}
	if !strings.Contains(stderr, "apply_patch") || strings.Contains(stderr, "Edit or Write") {
		t.Errorf("stderr = %q, want apply_patch and not Edit or Write", stderr)
	}
	for name, in := range map[string]string{
		"allowed command": `{"tool_name":"Bash","tool_input":{"command":"git status"}}`,
		"unreadable json": `not json`,
	} {
		if _, err := run(in); err != nil {
			t.Errorf("%s: err = %v, want nil", name, err)
		}
	}
}

func TestAgentGuardQuestion_Blocks(t *testing.T) {
	t.Parallel()
	c := newAgentGuardQuestionCmd()
	var stderr bytes.Buffer
	c.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"request_user_input","tool_input":{}}`))
	c.SetErr(&stderr)
	var exitErr *ExitError
	if err := c.Execute(); !errors.As(err, &exitErr) || exitErr.Code != guardBashExitCode {
		t.Fatalf("err = %v, want ExitError{%d}", err, guardBashExitCode)
	}
	if !strings.Contains(stderr.String(), "gx tickets ask") || !strings.Contains(stderr.String(), "request_user_input") {
		t.Errorf("stderr = %q, want the gx tickets ask pointer", stderr.String())
	}
}

func TestAgentGuardBash_ExitCodes(t *testing.T) {
	t.Parallel()
	run := func(stdin string) (string, error) {
		c := newAgentGuardBashCmd()
		var stderr bytes.Buffer
		c.SetIn(strings.NewReader(stdin))
		c.SetErr(&stderr)
		err := c.Execute()
		return stderr.String(), err
	}

	stderr, err := run(`{"tool_input":{"command":"sed -i s/a/b/ x.go"}}`)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != guardBashExitCode {
		t.Fatalf("err = %v, want ExitError{%d}", err, guardBashExitCode)
	}
	if !strings.Contains(stderr, "Edit or Write") {
		t.Errorf("stderr = %q, want a reason naming Edit/Write", stderr)
	}

	for name, in := range map[string]string{
		"allowed command": `{"tool_input":{"command":"git status"}}`,
		"unreadable json": `not json`,
	} {
		if _, err := run(in); err != nil {
			t.Errorf("%s: err = %v, want nil", name, err)
		}
	}
}
