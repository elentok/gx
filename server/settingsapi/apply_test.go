package settingsapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/config/registry"
)

func set(v string, key ...string) Change {
	return Change{Key: registry.Key(key...), Value: json.RawMessage(v)}
}

func remove(key ...string) Change { return Change{Key: registry.Key(key...), Remove: true} }

func TestApplySetAndRemoveKeepOrder(t *testing.T) {
	reg := registry.Default()
	doc := []byte(`{"disk-space":{"warn-below":"1G","clean-below":"1G"},"agent-runner":"pty"}`)

	out, err := Apply(reg, ScopeGlobal, doc, []Change{set(`"5m"`, "execution-queue", "waiting-slot-grace")})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"disk-space\": {\n    \"warn-below\": \"1G\",\n    \"clean-below\": \"1G\"\n  },\n  \"agent-runner\": \"pty\",\n  \"execution-queue\": {\n    \"waiting-slot-grace\": \"5m\"\n  }\n}\n"
	if string(out) != want {
		t.Fatalf("got %q", out)
	}

	out, err = Apply(reg, ScopeGlobal, out, []Change{remove("execution-queue", "waiting-slot-grace")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "execution-queue") || !strings.Contains(string(out), "agent-runner") {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEmptyCleanupCommandIsWritten(t *testing.T) {
	out, err := Apply(registry.Default(), ScopeGlobal, []byte(`{}`), []Change{set(`[]`, "disk-space", "cleanup-command")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"cleanup-command": []`) {
		t.Fatalf("got %q", out)
	}
}

func TestApplyRefusals(t *testing.T) {
	reg := registry.Default()
	docWithFull := []byte(`{}`)
	tests := []struct {
		name    string
		scope   Scope
		changes []Change
		key     string
	}{
		{"duration typo", ScopeGlobal, []Change{set(`"5mm"`, "execution-queue", "waiting-slot-grace")}, "execution-queue.waiting-slot-grace"},
		{"zero duration", ScopeGlobal, []Change{set(`"0s"`, "execution-queue", "waiting-slot-grace")}, "execution-queue.waiting-slot-grace"},
		{"bad size", ScopeGlobal, []Change{set(`"5x"`, "disk-space", "warn-below")}, "disk-space.warn-below"},
		{"enum non-member", ScopeGlobal, []Change{set(`"nope"`, "agent-runner")}, "agent-runner"},
		{"number below minimum", ScopeGlobal, []Change{set(`0`, "execution-queue", "max-agents")}, "execution-queue.max-agents"},
		{"unknown key", ScopeGlobal, []Change{set(`1`, "no-such")}, "no-such"},
		{"read-only in project", ScopeProject, []Change{set(`"x"`, "name")}, "name"},
		{"project-only in global", ScopeGlobal, []Change{set(`"main"`, "trunk")}, "trunk"},
		{"notifications in project", ScopeProject, []Change{set(`"x"`, "notifications", "telegram", "chat-id")}, "notifications.telegram.chat-id"},
		{"deep-read-cap in project", ScopeProject, []Change{set(`5`, "evolve.deep-read-cap")}, "evolve.deep-read-cap"},
		{"ignored agents sub-key in project", ScopeProject, []Change{set(`true`, "agents", "banners")}, "agents.banners"},
		{"checks without full", ScopeProject, []Change{set(`{"test":{"changed":"x"}}`, "checks")}, "checks"},
		{"two keys, one invalid", ScopeGlobal, []Change{
			set(`"5m"`, "execution-queue", "waiting-slot-grace"),
			set(`"5mm"`, "execution-queue", "waiting-slot-grace"),
		}, "execution-queue.waiting-slot-grace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Apply(reg, tt.scope, docWithFull, tt.changes)
			if out != nil {
				t.Fatalf("returned bytes %q", out)
			}
			r, ok := err.(*Refusal)
			if !ok {
				t.Fatalf("err = %v", err)
			}
			if len(r.Keys) != 1 || r.Keys[0].String() != tt.key || r.Reason == "" {
				t.Fatalf("refusal = %+v", r)
			}
		})
	}
}

func TestApplyChecksRefusalCarriesParseConfigMessage(t *testing.T) {
	_, err := Apply(registry.Default(), ScopeProject, []byte(`{}`), []Change{set(`{"test":{"changed":"x"}}`, "checks")})
	if err == nil || !strings.Contains(err.Error(), `"full" is required`) {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyOverriddenKeyUsesProjectPath(t *testing.T) {
	out, err := Apply(registry.Default(), ScopeProject, []byte(`{}`), []Change{set(`3`, "project", "max-agents")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"max-agents": 3`) || strings.Contains(string(out), "project") {
		t.Fatalf("got %q", out)
	}
}

func TestApplyProjectFileValidationRefusal(t *testing.T) {
	reg := registry.New(registry.Entry{Key: registry.Key("landing"), Type: registry.TypeString, ProjectOnly: true})
	out, err := Apply(reg, ScopeProject, []byte(`{}`), []Change{set(`"bogus"`, "landing")})
	if out != nil || err == nil || !strings.Contains(err.Error(), "landing") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestApplyGlobalValidateSettingRefusal(t *testing.T) {
	reg := registry.New(registry.Entry{Key: registry.Key("agent-runner"), Type: registry.TypeString})
	out, err := Apply(reg, ScopeGlobal, []byte(`{}`), []Change{set(`"bogus"`, "agent-runner")})
	if out != nil || err == nil || !strings.Contains(err.Error(), "agent-runner") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestApplyCombinedOnlyFailureNamesEveryKey(t *testing.T) {
	reg := registry.New(
		registry.Entry{Key: registry.Key("checks", "test", "changed"), Type: registry.TypeString, ProjectOnly: true},
		registry.Entry{Key: registry.Key("checks", "test", "full"), Type: registry.TypeString, ProjectOnly: true},
	)
	doc := []byte(`{"checks":{"test":{"full":"a"}}}`)
	out, err := Apply(reg, ScopeProject, doc, []Change{
		set(`"x"`, "checks", "test", "changed"),
		remove("checks", "test", "full"),
	})
	if out != nil {
		t.Fatalf("returned bytes %q", out)
	}
	r, ok := err.(*Refusal)
	if !ok || len(r.Keys) != 2 {
		t.Fatalf("err = %#v", err)
	}
}
