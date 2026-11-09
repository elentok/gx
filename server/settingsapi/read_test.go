package settingsapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/wire"
)

const (
	testToken   = "123456:SECRET-TOKEN-VALUE"
	testWebhook = "https://hooks.slack.example/SECRET-WEBHOOK"
)

func entryByKey(t *testing.T, resp wire.SettingsReadResponse, key string) wire.SettingsEntry {
	t.Helper()
	for _, e := range resp.Entries {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("no entry %s", key)
	return wire.SettingsEntry{}
}

func hasKey(resp wire.SettingsReadResponse, key string) bool {
	for _, e := range resp.Entries {
		if e.Key == key {
			return true
		}
	}
	return false
}

// readFixture builds a writer over a global file and one project "proj".
func readFixture(t *testing.T, global, project string) (w *Writer, repo string) {
	t.Helper()
	dir := t.TempDir()
	repo = filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	projDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gpath := filepath.Join(dir, "config.json")
	writeFile(t, gpath, global)
	writeFile(t, filepath.Join(projDir, config.ProjectFileName),
		strings.Replace(project, "REPO", repo, 1))
	w = newWriter(gpath)
	w.ProjectDir = func(string) (string, error) { return projDir, nil }
	return w, repo
}

func TestReadGlobalHidesSecretsAndShowsChatID(t *testing.T) {
	t.Parallel()
	w, _ := readFixture(t, `{
		"notifications":{"telegram":{"bot-token":"`+testToken+`","chat-id":"4242"},"slack":{"webhook-url":"`+testWebhook+`"}},
		"execution-queue":{"max-agents":3}}`, `{}`)
	resp, err := w.Read(Target{}, []string{"server.tcp-listen"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), testToken) || strings.Contains(string(raw), testWebhook) {
		t.Fatalf("secret leaked: %s", raw)
	}
	for _, key := range []string{"notifications.telegram.bot-token", "notifications.slack.webhook-url"} {
		e := entryByKey(t, resp, key)
		if !e.Set || e.Value != nil {
			t.Errorf("%s: set=%v value=%s", key, e.Set, e.Value)
		}
	}
	if e := entryByKey(t, resp, "notifications.telegram.chat-id"); string(e.Value) != `"4242"` {
		t.Errorf("chat-id = %s", e.Value)
	}
	if len(resp.PendingRestart) != 1 || resp.PendingRestart[0] != "server.tcp-listen" {
		t.Errorf("pending = %v", resp.PendingRestart)
	}
	for _, e := range w.Registry.All() {
		if !e.ProjectOnly && !hasKey(resp, e.Key.String()) {
			t.Errorf("global read misses %s", e.Key)
		}
	}
}

func TestReadDistinguishesSetFromDefault(t *testing.T) {
	t.Parallel()
	w, _ := readFixture(t, `{"execution-queue":{"max-agents":3}}`, `{}`)
	resp, err := w.Read(Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	set := entryByKey(t, resp, "execution-queue.max-agents")
	if !set.Set || string(set.Value) != "3" || set.Default == nil {
		t.Errorf("set entry = %+v", set)
	}
	unset := entryByKey(t, resp, "execution-queue.spin-window")
	if unset.Set || unset.Value != nil || unset.Default == nil {
		t.Errorf("unset entry = %+v", unset)
	}
	if e := entryByKey(t, resp, "server.tcp-listen"); !e.Restart || !e.Risky || e.ConfirmText == "" {
		t.Errorf("flags lost: %+v", e)
	}
}

func TestReadProjectListsOnlyProjectKeys(t *testing.T) {
	t.Parallel()
	w, _ := readFixture(t, `{"execution-queue":{"max-agents":6},"disk-space":{"cleanup-command":["x"]}}`,
		`{"name":"proj","repo":"REPO","vcs":"git","max-agents":2,
		"notifications":{"telegram":{"chat-id":"9"}}}`)
	resp, err := w.Read(Target{Project: "proj"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range resp.Entries {
		def, _ := w.Registry.Lookup(registry.KeyPath(e.Path))
		if !def.AppliesToProject() {
			t.Errorf("project read lists %s", e.Key)
		}
		if def.Section == registry.SectionNotifications {
			t.Errorf("project read lists notifications key %s", e.Key)
		}
	}
	for _, key := range []string{"evolve.deep-read-cap", "notifications.telegram.chat-id", "server.tcp-listen"} {
		if hasKey(resp, key) {
			t.Errorf("project read lists %s", key)
		}
	}
	if resp.Project == nil || resp.Project.Name != "proj" || resp.Project.VCS != "git" {
		t.Errorf("project = %+v", resp.Project)
	}
	if first, last := resp.Entries[0].Section, resp.Entries[len(resp.Entries)-1].Section; first != "Project" || last != "Checks" {
		t.Errorf("entries run from %q to %q, want Project to Checks", first, last)
	}

	cap := entryByKey(t, resp, "project.max-agents")
	if !cap.Set || string(cap.Value) != "2" || cap.GlobalKey != "execution-queue.max-agents" || string(cap.GlobalValue) != "6" {
		t.Errorf("max-agents = %+v", cap)
	}
	if cap.Label != "Max live agents for this project" || cap.Override != "cap" {
		t.Errorf("max-agents label/override = %q/%q", cap.Label, cap.Override)
	}
	over := entryByKey(t, resp, "disk-space.cleanup-command")
	if over.Set || string(over.GlobalValue) != `["x"]` {
		t.Errorf("cleanup-command = %+v", over)
	}
}

func TestReadProjectChecksRepoFileFlag(t *testing.T) {
	t.Parallel()
	w, repo := readFixture(t, `{}`, `{"name":"proj","repo":"REPO"}`)
	resp, err := w.Read(Target{Project: "proj"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Project.RepoChecksFile {
		t.Error("repo file flagged before it exists")
	}
	if err := os.MkdirAll(filepath.Join(repo, ".gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, ".gx", "checks.json"), `{}`)
	resp, err = w.Read(Target{Project: "proj"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Project.RepoChecksFile {
		t.Error("repo file not flagged")
	}
}
