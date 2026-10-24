package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

const pkgJSON = `{"name":"x"}`

// installRepo makes a committed repo holding files (path -> content), then
// writes untracked on top of it uncommitted. Directories are created as needed.
func installRepo(t *testing.T, files, untracked map[string]string) string {
	t.Helper()
	repo := testutil.TempRepo(t)
	write := func(set map[string]string) {
		for name, content := range set {
			testutil.Mkdir(t, filepath.Dir(filepath.Join(repo, name)))
			testutil.WriteFile(t, repo, name, content)
		}
	}
	write(files)
	testutil.CommitAll(t, repo, "add install files")
	write(untracked)
	return repo
}

// stubManagers puts a stub for each named package manager into one bin dir and
// returns it with the log all of them append to.
func stubManagers(t *testing.T, exitCode int, names ...string) (binDir, logPath string) {
	t.Helper()
	logPath = filepath.Join(t.TempDir(), "invocations.log")
	binDir = t.TempDir()
	for _, name := range names {
		writeStub(t, binDir, name, logPath, exitCode)
	}
	return binDir, logPath
}

func readLog(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestInstallDependencies_NoTrackedLockfile_IsNoOp(t *testing.T) {
	t.Parallel()
	repo := installRepo(t,
		map[string]string{"package.json": pkgJSON, "go.mod": "module x\n"},
		map[string]string{"package-lock.json": "{}"},
	)
	binDir, logPath := stubManagers(t, 0, "npm")

	commands, err := installDependenciesWith(repo, binDir, "")
	if err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if len(commands) != 0 {
		t.Errorf("commands = %v, want none: the only lockfile is untracked", commands)
	}
	if got := readLog(t, logPath); got != nil {
		t.Errorf("stub invoked: %v", got)
	}
}

func TestInstallDependencies_EachManager_RunsExpectedCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		files       map[string]string
		wantCommand string
		wantBin     string
	}{
		{"npm", map[string]string{"package-lock.json": "{}"}, "npm ci", "npm"},
		{"pnpm", map[string]string{"pnpm-lock.yaml": ""}, "pnpm install --frozen-lockfile", "pnpm"},
		{"yarn classic lockfile", map[string]string{"yarn.lock": "# yarn lockfile v1\n"}, "yarn install --frozen-lockfile", "yarn"},
		{"yarn berry lockfile", map[string]string{"yarn.lock": "__metadata:\n  version: 8\n"}, "yarn install --immutable", "yarn"},
		{"bun.lock", map[string]string{"bun.lock": ""}, "bun install --frozen-lockfile", "bun"},
		{"bun.lockb", map[string]string{"bun.lockb": ""}, "bun install --frozen-lockfile", "bun"},
		{"bun.lock and bun.lockb are one manager", map[string]string{"bun.lock": "", "bun.lockb": ""}, "bun install --frozen-lockfile", "bun"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"package.json": pkgJSON}
			for k, v := range c.files {
				files[k] = v
			}
			repo := installRepo(t, files, nil)
			binDir, logPath := stubManagers(t, 0, c.wantBin)

			commands, err := installDependenciesWith(repo, binDir, "")
			if err != nil {
				t.Fatalf("installDependenciesWith() error = %v", err)
			}
			if want := []string{repo + ": " + c.wantCommand}; !equalStrings(commands, want) {
				t.Errorf("commands = %v, want %v", commands, want)
			}
			if got := readLog(t, logPath); len(got) != 1 {
				t.Errorf("invocations = %v, want exactly one", got)
			}
		})
	}
}

func TestInstallDependencies_PackageManagerField_BeatsLockfile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		packageManager string
		lockfile       string
		lockContent    string
		wantCommand    string
		wantBin        string
	}{
		{"pnpm over npm lockfile", "pnpm@9.1.0", "package-lock.json", "{}", "pnpm install --frozen-lockfile", "pnpm"},
		{"yarn 1 over berry lockfile", "yarn@1.22.19", "yarn.lock", "__metadata:\n", "yarn install --frozen-lockfile", "yarn"},
		{"yarn 4 over classic lockfile", "yarn@4.1.0", "yarn.lock", "# yarn lockfile v1\n", "yarn install --immutable", "yarn"},
		{"pnpm with hash", "pnpm@9.1.0+sha256.abc", "package-lock.json", "{}", "pnpm install --frozen-lockfile", "pnpm"},
		{"yarn berry with hash", "yarn@3.6.0+sha224.abc", "yarn.lock", "", "yarn install --immutable", "yarn"},
		{"bun", "bun@1.1.0", "package-lock.json", "{}", "bun install --frozen-lockfile", "bun"},
		{"npm", "npm@10.2.0", "pnpm-lock.yaml", "", "npm ci", "npm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			repo := installRepo(t, map[string]string{
				"package.json": `{"packageManager":"` + c.packageManager + `"}`,
				c.lockfile:     c.lockContent,
			}, nil)
			binDir, _ := stubManagers(t, 0, c.wantBin)

			commands, err := installDependenciesWith(repo, binDir, "")
			if err != nil {
				t.Fatalf("installDependenciesWith() error = %v", err)
			}
			if want := []string{repo + ": " + c.wantCommand}; !equalStrings(commands, want) {
				t.Errorf("commands = %v, want %v", commands, want)
			}
		})
	}
}

func TestInstallDependencies_RootAndSubdirectory_InstallEachInItsOwnDirectory(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{
		"package.json":                 pkgJSON,
		"package-lock.json":            "{}",
		"web/package.json":             pkgJSON,
		"web/pnpm-lock.yaml":           "",
		"untracked-dir/package.json":   pkgJSON,
		"tools/cli/package.json":       pkgJSON,
		"tools/cli/yarn.lock":          "__metadata:\n",
		"tools/cli/node_modules/x.txt": "ignored",
	}, map[string]string{"untracked-dir/bun.lock": ""})
	binDir, logPath := stubManagers(t, 0, "npm", "pnpm", "yarn", "bun")

	commands, err := installDependenciesWith(repo, binDir, "")
	if err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	want := []string{
		repo + ": npm ci",
		filepath.Join(repo, "tools", "cli") + ": yarn install --immutable",
		filepath.Join(repo, "web") + ": pnpm install --frozen-lockfile",
	}
	if !equalStrings(commands, want) {
		t.Errorf("commands = %v, want %v", commands, want)
	}

	logged := strings.Join(readLog(t, logPath), "\n")
	for _, dir := range []string{repo, filepath.Join(repo, "tools", "cli"), filepath.Join(repo, "web")} {
		if !strings.Contains(logged, "cwd="+dir+" ") {
			t.Errorf("log %q has no install run in %s", logged, dir)
		}
	}
}

func TestInstallDependencies_RunsWithCITrue(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	binDir, logPath := stubManagers(t, 0, "npm")

	if _, err := installDependenciesWith(repo, binDir, ""); err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if got := readLog(t, logPath); len(got) != 1 || !strings.Contains(got[0], " CI=true ") {
		t.Errorf("log = %v, want one run with CI=true", got)
	}
}

func TestInstallDependencies_Failures_ExplainThemselves(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		files   map[string]string
		wantErr string
		noRepo  bool
	}{
		{
			name:    "missing package.json",
			files:   map[string]string{"package-lock.json": "{}"},
			wantErr: "reading package.json: ",
		},
		{
			name:    "invalid package.json",
			files:   map[string]string{"package.json": "{nope", "package-lock.json": "{}"},
			wantErr: "reading package.json: ",
		},
		{
			name:    "unknown packageManager",
			files:   map[string]string{"package.json": `{"packageManager":"deno@2.0.0"}`, "package-lock.json": "{}"},
			wantErr: `unsupported packageManager "deno@2.0.0"`,
		},
		{
			name:    "unparseable packageManager",
			files:   map[string]string{"package.json": `{"packageManager":"npm"}`, "package-lock.json": "{}"},
			wantErr: `unsupported packageManager "npm"`,
		},
		{
			name:    "packageManager with non-numeric yarn major",
			files:   map[string]string{"package.json": `{"packageManager":"yarn@berry"}`, "yarn.lock": ""},
			wantErr: `unsupported packageManager "yarn@berry"`,
		},
		{
			name:    "packageManager with non-numeric pnpm major",
			files:   map[string]string{"package.json": `{"packageManager":"pnpm@x"}`, "pnpm-lock.yaml": ""},
			wantErr: `unsupported packageManager "pnpm@x"`,
		},
		{
			name:    "packageManager with empty version",
			files:   map[string]string{"package.json": `{"packageManager":"npm@"}`, "package-lock.json": "{}"},
			wantErr: `unsupported packageManager "npm@"`,
		},
		{
			name:    "two managers without packageManager",
			files:   map[string]string{"package.json": pkgJSON, "package-lock.json": "{}", "pnpm-lock.yaml": ""},
			wantErr: "package-lock.json, pnpm-lock.yaml",
		},
		{
			name:    "git ls-files fails",
			noRepo:  true,
			wantErr: "git ls-files",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var repo string
			if c.noRepo {
				repo = t.TempDir()
			} else {
				repo = installRepo(t, c.files, nil)
			}
			binDir, logPath := stubManagers(t, 0, "npm", "pnpm")

			commands, err := installDependenciesWith(repo, binDir, "")
			if err == nil {
				t.Fatalf("installDependenciesWith() = %v, nil error; want failure", commands)
			}
			prefix := "installing dependencies in " + repo + ": "
			if !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q, want prefix %q containing %q", err, prefix, c.wantErr)
			}
			if got := readLog(t, logPath); got != nil {
				t.Errorf("stub invoked despite failure: %v", got)
			}
		})
	}
}

func TestInstallDependencies_FailingCommand_ReportsCommandAndOutput(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	binDir, _ := stubManagers(t, 1, "npm")

	_, err := installDependenciesWith(repo, binDir, "")
	if err == nil {
		t.Fatal("installDependenciesWith() error = nil, want failure surfaced")
	}
	want := "installing dependencies in " + repo + ": npm ci: boom"
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err, want)
	}
}

func writeScriptStub(t *testing.T, binDir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}
}

func TestInstallDependencies_HungInstall_TimesOut(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	binDir := t.TempDir()
	writeScriptStub(t, binDir, "npm", "sleep 5")

	start := time.Now()
	_, err := installDependenciesWithin(repo, binDir, "", 200*time.Millisecond)
	want := "installing dependencies in " + repo + ": npm ci: timed out after 5m"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("took %v, want the stub killed near the budget", elapsed)
	}
}

func TestInstallDependencies_TwoDirectories_ShareOneBudget(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{
		"package.json":          pkgJSON,
		"package-lock.json":     "{}",
		"web/package.json":      pkgJSON,
		"web/package-lock.json": "{}",
	}, nil)
	binDir := t.TempDir()
	writeScriptStub(t, binDir, "npm", "sleep 0.4")

	// Each install alone fits the budget; together they do not.
	_, err := installDependenciesWithin(repo, binDir, "", 750*time.Millisecond)
	want := "installing dependencies in " + filepath.Join(repo, "web") + ": npm ci: timed out after 5m"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestInstallDependencies_LongFailure_KeepsTailAndLogsFullOutput(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	binDir := t.TempDir()
	writeScriptStub(t, binDir, "npm", "echo FIRST-LINE; i=0; while [ $i -lt 500 ]; do echo \"filler line $i\"; i=$((i+1)); done; echo LAST-LINE; exit 1")
	logDir := filepath.Join(t.TempDir(), "logs")

	_, err := installDependenciesWith(repo, binDir, logDir)
	if err == nil {
		t.Fatal("error = nil, want failure")
	}
	msg := err.Error()
	if strings.Contains(msg, "FIRST-LINE") || !strings.Contains(msg, "LAST-LINE") {
		t.Errorf("error = %q, want only the output tail", msg)
	}
	if len(msg) > installTailBytes+500 {
		t.Errorf("error is %d bytes, want it trimmed", len(msg))
	}
	logs, _ := filepath.Glob(filepath.Join(logDir, "*.log"))
	if len(logs) != 1 || !strings.Contains(msg, "(full output: "+logs[0]+")") {
		t.Fatalf("logs = %v, want one named in %q", logs, msg)
	}
	full, err := os.ReadFile(logs[0])
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if !strings.Contains(string(full), "FIRST-LINE") || !strings.Contains(string(full), "LAST-LINE") {
		t.Errorf("log lacks the full output")
	}
}

func TestInstallDependencies_Success_WritesNoLog(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	binDir, _ := stubManagers(t, 0, "npm")
	logDir := filepath.Join(t.TempDir(), "logs")

	if _, err := installDependenciesWith(repo, binDir, logDir); err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if _, err := os.Stat(logDir); !os.IsNotExist(err) {
		t.Errorf("log dir stat err = %v, want it absent after a successful install", err)
	}
}

func TestInstallDependencies_ManagerNotOnPath_Fails(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)

	_, err := installDependenciesWith(repo, t.TempDir(), "")
	if err == nil || !strings.HasPrefix(err.Error(), "installing dependencies in "+repo+": npm ci: ") {
		t.Fatalf("error = %v, want npm ci lookup failure", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
