package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire/ops"
)

type rootsFixture struct {
	h      *servertest.Harness
	config string
	home   string
}

func startRoots(t *testing.T) rootsFixture {
	t.Helper()
	config := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithConfigFile(config))
	return rootsFixture{h: h, config: config, home: h.Env("HOME")}
}

// run runs `gx server files-roots <args> --json` and returns the decoded result.
func (f rootsFixture) run(t *testing.T, args ...string) (ops.HostedRootsResult, error) {
	t.Helper()
	var out bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:       []string{"HOME=" + f.home},
		Stdout:    &out,
		Stderr:    &bytes.Buffer{},
		NewClient: func() (*apiclient.Client, error) { return f.h.Client, nil },
	}}
	err := execute(t, append([]string{"server", "files-roots"}, append(args, "--json")...), d)
	var res ops.HostedRootsResult
	if out.Len() > 0 {
		if jerr := json.NewDecoder(&out).Decode(&res); jerr != nil {
			t.Fatalf("decode %q: %v", out.String(), jerr)
		}
	}
	return res, err
}

func sourceOf(res ops.HostedRootsResult, path string) ops.HostedRootSource {
	for _, r := range res.Roots {
		if r.Path == path {
			return r.Source
		}
	}
	return ""
}

func (f rootsFixture) found(t *testing.T, file string) bool {
	t.Helper()
	res, err := f.h.Client.Web.ResolveHostedFiles(context.Background(), ops.HostedResolveRequest{Candidates: []string{file}})
	if err != nil {
		t.Fatal(err)
	}
	return res.Files[0].Verdict == ops.HostedFound
}

func TestFilesRoots_AddListPersistRemove(t *testing.T) {
	f := startRoots(t)
	dir := filepath.Join(servertest.OutsideTempDir(t), "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f.found(t, file) {
		t.Fatal("file hosted before its folder was added")
	}

	if _, err := f.run(t, "add", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "add", dir); err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "add", nested); err != nil {
		t.Fatalf("nested add: %v", err)
	}
	list, err := f.run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceOf(list, dir); got != ops.HostedRootConfig {
		t.Errorf("source of %s = %q, want config", dir, got)
	}
	n := 0
	for _, r := range list.Roots {
		if r.Path == dir {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%s listed %d times, want 1", dir, n)
	}
	builtin := 0
	for _, r := range list.Roots {
		if r.Source == ops.HostedRootBuiltIn {
			builtin++
		}
	}
	if builtin == 0 {
		t.Error("no built-in roots listed")
	}
	if cfg, _ := os.ReadFile(f.config); !strings.Contains(string(cfg), dir) {
		t.Errorf("config file %q does not hold %s", cfg, dir)
	}
	if !f.found(t, file) {
		t.Error("file in added folder is not found without a restart")
	}

	f.h.Restart(t)
	// The harness restarts with the settings it was built with; gx's real start
	// loads the config file, which this reload stands in for.
	f.h.Server.Settings().Reload(settings.TriggerCommand)
	list, err = f.run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceOf(list, dir); got != ops.HostedRootConfig {
		t.Errorf("after restart source of %s = %q, want config", dir, got)
	}

	if _, err := f.run(t, "remove", nested); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, "remove", dir); err != nil {
		t.Fatal(err)
	}
	if f.found(t, file) {
		t.Error("file still hosted after its folder was removed")
	}
}

func TestFilesRoots_RefusalsHaveDistinctReasons(t *testing.T) {
	f := startRoots(t)
	notFolder := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(notFolder, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(f.home, "projects")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := f.run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	var builtin string
	for _, r := range list.Roots {
		if r.Source == ops.HostedRootBuiltIn {
			builtin = r.Path
			break
		}
	}
	cases := []struct {
		name   string
		args   []string
		reason string
	}{
		{"slash", []string{"add", "/"}, ops.RootReasonIsSlash},
		{"home", []string{"add", "~"}, ops.RootReasonIsHome},
		{"missing", []string{"add", filepath.Join(t.TempDir(), "nope")}, ops.RootReasonMissing},
		{"file", []string{"add", notFolder}, ops.RootReasonNotFolder},
		{"remove built-in", []string{"remove", builtin}, ops.RootReasonBuiltIn},
		{"remove unlisted", []string{"remove", sub}, ops.RootReasonNotAdded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := f.run(t, tc.args...)
			if err == nil {
				t.Fatal("want a non-zero exit")
			}
			if res.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", res.Reason, tc.reason)
			}
		})
	}
	if _, err := f.run(t, "add", "~/projects"); err != nil {
		t.Errorf("a subfolder of home is refused: %v", err)
	}
}
