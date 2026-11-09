package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestSettingsScenario(t *testing.T) {
	t.Parallel()
	r := start(t, "settings")

	if code := r.Get(t, r.WebURL+"/v1/snapshot", true); code != 200 {
		t.Errorf("GET /v1/snapshot = %d, want 200", code)
	}

	raw, err := os.ReadFile(r.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	section := func(path ...string) map[string]any {
		var node any = cfg
		for _, p := range path {
			m, _ := node.(map[string]any)
			node = m[p]
		}
		m, _ := node.(map[string]any)
		return m
	}
	for name, got := range map[string]any{
		"execution-queue": section("execution-queue")["max-released-agents"],
		"budget":          section("budget")["soft-limit"],
		"notifications":   section("notifications", "telegram")["bot-token"],
		"server":          section("server")["recently-finished"],
		"agents":          section("agents", "sounds")["enabled"],
		"evolve":          cfg["evolve.deep-read-cap"],
		"recovery":        section("recovery")["notify-hold"],
		"ticket-store":    section("ticket-store")["commit-debounce"],
	} {
		if got == nil {
			t.Errorf("config section %s has no non-default value:\n%s", name, raw)
		}
	}
	if section("notifications", "telegram")["chat-id"] == nil {
		t.Error("telegram chat-id not set")
	}
	if _, ok := section("notifications", "slack")["webhook-url"]; ok {
		t.Error("slack webhook-url is set, want it unset")
	}
	if _, ok := section("server", "web")["dev-frontend"]; ok {
		t.Error("risky server.web.dev-frontend is set, want its default")
	}

	store := filepath.Join(r.Root(), "data", "gx", "tickets")
	project := func(name string) (string, map[string]any) {
		body, err := os.ReadFile(filepath.Join(store, name, "project.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("%s project.json: %v\n%s", name, err, body)
		}
		return string(body), m
	}

	_, demo := project("demo")
	for _, key := range []string{"max-agents", "execution-queue", "agents", "disk-space", "checks"} {
		if demo[key] == nil {
			t.Errorf("demo project.json has no %q", key)
		}
	}
	if demo["evolve"] != false {
		t.Errorf("demo evolve = %v, want false", demo["evolve"])
	}
	slow := demo["checks"].(map[string]any)["test"].(map[string]any)["limits"].(map[string]any)["slow"]
	if m, ok := slow.(map[string]any); !ok || len(m) < 2 {
		t.Errorf("demo checks slow = %v, want a per-tool map", slow)
	}

	notesBody, _ := project("notes")
	if !strings.Contains(notesBody, spec.NotesNotifications) {
		t.Errorf("notes project.json lost its notifications block:\n%s", notesBody)
	}

	_, plain := project("plain")
	for _, key := range []string{"max-agents", "evolve", "agents", "execution-queue", "notifications"} {
		if _, ok := plain[key]; ok {
			t.Errorf("plain project.json has %q, want a plain project", key)
		}
	}

	var page struct {
		PendingRestart []string `json:"pending_restart"`
	}
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "settings", "--json")), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.PendingRestart) != 1 || page.PendingRestart[0] != "agent-runner" {
		t.Errorf("pending restart = %v, want [agent-runner]", page.PendingRestart)
	}
}
