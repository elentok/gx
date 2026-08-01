package ralphloop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// stubBin writes an executable script named name into a fresh directory and
// prepends that directory to PATH for the duration of the test, so
// InstallDependencies's exec.Command calls resolve to it instead of the real
// package manager. The script appends its own invocation ("name arg1 arg2 ...")
// as one line to logPath, and exits with exitCode.
func stubBin(t *testing.T, name string, logPath string, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$0 $*\" >> " + logPath + "\nexit " + strconv.Itoa(exitCode) + "\n"
	scriptPath := filepath.Join(dir, name)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInstallDependencies_NoMarker_SkipsSilently(t *testing.T) {
	dir := t.TempDir()

	command, err := InstallDependencies(dir)
	if err != nil {
		t.Fatalf("InstallDependencies() error = %v", err)
	}
	if command != "" {
		t.Errorf("command = %q, want empty (no marker matched)", command)
	}
}

func TestInstallDependencies_GoModOnly_SkipsSilently(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	command, err := InstallDependencies(dir)
	if err != nil {
		t.Fatalf("InstallDependencies() error = %v", err)
	}
	if command != "" {
		t.Errorf("command = %q, want empty: go.mod alone should not trigger an install step (go build/test populate the module cache lazily)", command)
	}
}

func TestInstallDependencies_EachMarker_RunsExpectedCommand(t *testing.T) {
	cases := []struct {
		marker      string
		wantCommand string
		wantBin     string
	}{
		{"package-lock.json", "npm ci", "npm"},
		{"pnpm-lock.yaml", "pnpm install --frozen-lockfile", "pnpm"},
		{"yarn.lock", "yarn install --frozen-lockfile", "yarn"},
		{"poetry.lock", "poetry install", "poetry"},
		{"uv.lock", "uv sync", "uv"},
	}

	for _, c := range cases {
		t.Run(c.marker, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.marker), []byte(""), 0644); err != nil {
				t.Fatalf("WriteFile %s: %v", c.marker, err)
			}
			logPath := filepath.Join(t.TempDir(), "invocations.log")
			stubBin(t, c.wantBin, logPath, 0)

			command, err := InstallDependencies(dir)
			if err != nil {
				t.Fatalf("InstallDependencies() error = %v", err)
			}
			if command != c.wantCommand {
				t.Errorf("command = %q, want %q", command, c.wantCommand)
			}

			logged, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("stub was not invoked: %v", err)
			}
			if !strings.Contains(string(logged), c.wantBin) {
				t.Errorf("invocation log = %q, want it to mention %q", logged, c.wantBin)
			}
		})
	}
}

func TestInstallDependencies_MarkerPrecedence_NpmWinsOverPnpm(t *testing.T) {
	dir := t.TempDir()
	for _, marker := range []string{"package-lock.json", "pnpm-lock.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, marker), []byte(""), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", marker, err)
		}
	}
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	stubBin(t, "npm", logPath, 0)

	command, err := InstallDependencies(dir)
	if err != nil {
		t.Fatalf("InstallDependencies() error = %v", err)
	}
	if command != "npm ci" {
		t.Errorf("command = %q, want npm ci to win when both markers are present", command)
	}
}

func TestInstallDependencies_CommandFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile package-lock.json: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	stubBin(t, "npm", logPath, 1)

	command, err := InstallDependencies(dir)
	if err == nil {
		t.Fatal("InstallDependencies() error = nil, want failure surfaced")
	}
	if command != "npm ci" {
		t.Errorf("command = %q, want npm ci returned alongside the error", command)
	}
}
