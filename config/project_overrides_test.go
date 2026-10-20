package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ProjectFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReadProjectFile_RejectsNonWhitelistedKeysByName(t *testing.T) {
	cases := map[string]string{
		"use-nerdfont-icons":                   `{"use-nerdfont-icons": true}`,
		"budget":                               `{"budget": {"soft-limit": 1}}`,
		"bogus":                                `{"bogus": 1}`,
		"execution-queue.max-concurrent-epics": `{"execution-queue": {"max-concurrent-epics": 3}}`,
		"execution-queue.max-agents":           `{"execution-queue": {"max-agents": 3}}`,
	}
	for key, body := range cases {
		_, err := ReadProjectFile(writeProjectJSON(t, body))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: want error naming key, got %v", key, err)
		}
	}
}

func TestReadProjectFile_AcceptsWhitelist(t *testing.T) {
	dir := writeProjectJSON(t, `{
		"$schema": "s",
		"name": "p", "repo": "/r", "vcs": "none", "trunk": "main", "landing": "on-done",
		"auto-ff-merge": true, "max-agents": 2,
		"agents": {"claude": {"model": "m"}},
		"skills": {"implement": "s"},
		"notifications": {"slack": {"webhook-url": "u"}},
		"execution-queue": {"max-agents-per-epic": 4}
	}`)
	if _, err := ReadProjectFile(dir); err != nil {
		t.Fatal(err)
	}
}

func TestParseProjectFile_RejectsBadTypes(t *testing.T) {
	if _, err := ParseProjectFile([]byte(`{"max-agents": "two"}`)); err == nil || !strings.Contains(err.Error(), "max-agents") {
		t.Errorf("want error naming max-agents, got %v", err)
	}
}

func TestReadProjectFile_UnknownEnumsLoadWithDefaults(t *testing.T) {
	pf, err := ReadProjectFile(writeProjectJSON(t, `{"landing": "bogus", "vcs": "hg"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := pf.LandingPolicy(); got != LandingOnLanded {
		t.Errorf("landing = %q, want default %q", got, LandingOnLanded)
	}
}

func TestValidateProjectFile_RejectsBadEnums(t *testing.T) {
	cases := map[string]string{
		"landing": `{"landing": "pr"}`,
		"vcs":     `{"vcs": "hg"}`,
	}
	for key, body := range cases {
		pf, err := ParseProjectFile([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateProjectFile(pf); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: want error naming it, got %v", key, err)
		}
	}
	pf, _ := ParseProjectFile([]byte(`{"vcs": "none", "landing": "on-done"}`))
	if err := ValidateProjectFile(pf); err != nil {
		t.Errorf("valid enums rejected: %v", err)
	}
}

func TestWithProject_NotificationsReplaceTheGlobalBlock(t *testing.T) {
	global := Config{Notifications: NotificationsConfig{
		Telegram: TelegramConfig{BotToken: "t", ChatID: "c"},
		Slack:    SlackConfig{WebhookURL: "g"},
	}}
	pf, err := ReadProjectFile(writeProjectJSON(t, `{"notifications": {"slack": {"webhook-url": "u"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := global.WithProject(pf).Notifications
	if got.Slack.WebhookURL != "u" || got.Telegram.BotToken != "" {
		t.Errorf("notifications = %+v, want only the project's slack", got)
	}
}

func TestWithProject_MaxOneOffAgents(t *testing.T) {
	global := Config{ExecutionQueue: DefaultExecutionQueueConfig()}
	global.ExecutionQueue.MaxOneOffAgents = 6
	for body, want := range map[string]int{
		`{"execution-queue": {"max-one-off-agents": 2}}`: 2,
		`{"execution-queue": {"max-one-off-agents": 0}}`: 1,
		`{}`: 6,
	} {
		pf, err := ReadProjectFile(writeProjectJSON(t, body))
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if got := global.WithProject(pf).ExecutionQueue.MaxOneOffAgents; got != want {
			t.Errorf("%s: MaxOneOffAgents = %d, want %d", body, got, want)
		}
	}
}

func TestWithProject_OverridesOnlyThatCopy(t *testing.T) {
	global := Config{
		ExecutionQueue: DefaultExecutionQueueConfig(),
		Agents:         DefaultAgentsConfig(),
		Skills:         DefaultSkillsConfig(),
		Notifications:  DefaultNotificationsConfig(),
	}
	pf, err := ReadProjectFile(writeProjectJSON(t, `{
		"agents": {"claude": {"model": "opus"}},
		"skills": {"implement": "mine"},
		"notifications": {"slack": {"webhook-url": "u"}},
		"execution-queue": {"max-agents-per-epic": 7}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	got := global.WithProject(pf)
	if got.Agents.Claude.Model != "opus" || got.Agents.Claude.Effort != global.Agents.Claude.Effort {
		t.Errorf("agents = %+v", got.Agents.Claude)
	}
	if got.Skills.Implement != "mine" || len(got.Skills.CodeReview) != len(global.Skills.CodeReview) {
		t.Errorf("skills = %+v", got.Skills)
	}
	if got.Notifications.Slack.WebhookURL != "u" {
		t.Errorf("notifications = %+v", got.Notifications)
	}
	if got.ExecutionQueue.MaxConcurrentTicketsPerEpic != 7 {
		t.Errorf("per-epic = %d", got.ExecutionQueue.MaxConcurrentTicketsPerEpic)
	}
	if global.Agents.Claude.Model == "opus" || global.ExecutionQueue.MaxConcurrentTicketsPerEpic == 7 {
		t.Error("global config was mutated")
	}
}
