package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

func TestExecute_ConfigEdit_RequiresEditor(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
		getenv: func(string) string { return "" },
	}

	err := execute([]string{"config", "edit"}, d)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "$EDITOR is not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ConfigEdit_RunsEditor(t *testing.T) {
	var stdout bytes.Buffer
	var gotEditor, gotPath string
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		initConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
		getenv: func(k string) string {
			if k == "EDITOR" {
				return "vim"
			}
			return ""
		},
		runEditor: func(editor, path string, _ io.Reader, _, _ io.Writer) error {
			gotEditor = editor
			gotPath = path
			return nil
		},
	}

	if err := execute([]string{"config", "edit"}, d); err != nil {
		t.Fatalf("execute config edit: %v", err)
	}
	if gotEditor != "vim" {
		t.Fatalf("editor = %q, want %q", gotEditor, "vim")
	}
	if gotPath == "" {
		t.Fatal("expected non-empty config path")
	}
}

func TestExecute_ConfigDefaults_PrintsJSON(t *testing.T) {
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
	}
	if err := execute([]string{"config", "defaults"}, d); err != nil {
		t.Fatalf("execute config defaults: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "use-nerdfont-icons") {
		t.Fatalf("expected config key in output, got: %q", out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestExecute_ConfigShow_PrintsJSON(t *testing.T) {
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Default(), nil
		},
	}
	if err := execute([]string{"config", "show"}, d); err != nil {
		t.Fatalf("execute config show: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "use-nerdfont-icons") {
		t.Fatalf("expected config key in output, got: %q", out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestExecute_ConfigShow_PropagatesError(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Config{}, errors.New("load failed")
		},
	}
	err := execute([]string{"config", "show"}, d)
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestRelativeDate_Zero(t *testing.T) {
	got := relativeDate(time.Time{})
	if got != "unknown time" {
		t.Fatalf("relativeDate(zero) = %q, want %q", got, "unknown time")
	}
}

func TestRelativeDate_NonZero(t *testing.T) {
	got := relativeDate(time.Now().Add(-time.Hour))
	if got == "" || got == "unknown time" {
		t.Fatalf("relativeDate(now-1h) = %q, expected a non-empty relative string", got)
	}
}

func TestRunEditorCommand_Success(t *testing.T) {
	path := t.TempDir() + "/file.txt"
	err := runEditorCommand("touch", path, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("runEditorCommand touch: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatal("expected file to be created by touch")
	}
}

func TestRunEditorCommand_Failure(t *testing.T) {
	err := runEditorCommand("false", "/irrelevant", bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error from false command")
	}
}

func TestRunEditorCommand_MultiWordEditor(t *testing.T) {
	var stdout bytes.Buffer
	// Verify a multi-word $EDITOR (e.g. "code --wait") gets split correctly.
	err := runEditorCommand("echo hello", "/dev/null", bytes.NewBuffer(nil), &stdout, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("runEditorCommand multi-word: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("expected hello in output, got: %q", stdout.String())
	}
}
