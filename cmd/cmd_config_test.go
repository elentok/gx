package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

func TestExecute_ConfigEdit_RequiresEditor(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if !strings.Contains(out, "max-concurrent-tickets-per-epic") || !strings.Contains(out, "max-concurrent-epics") {
		t.Fatalf("expected execution queue config keys in output, got: %q", out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestSettingsFromConfigIncludesExecutionQueueLimits(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic = 3
	cfg.ExecutionQueue.MaxConcurrentEpics = 4

	settings := settingsFromConfig(cfg)
	if got := settings.MaxConcurrentTicketsPerEpic(); got != 3 {
		t.Fatalf("MaxConcurrentTicketsPerEpic() = %d, want 3", got)
	}
	if got := settings.MaxConcurrentEpics(); got != 4 {
		t.Fatalf("MaxConcurrentEpics() = %d, want 4", got)
	}
}

func TestExecute_ConfigShow_PrintsJSON(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestExecute_ConfigTestNotifications_NoneConfiguredPrintsNoticeAndSucceeds(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Default(), nil
		},
	}
	if err := execute([]string{"config", "test-notifications"}, d); err != nil {
		t.Fatalf("execute config test-notifications: %v", err)
	}
	if !strings.Contains(stdout.String(), "no notification service configured") {
		t.Fatalf("expected notice about no configured service, got: %q", stdout.String())
	}
}

func TestExecute_ConfigTestNotifications_SlackConfiguredSendsAndReportsSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			cfg := config.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}
	if err := execute([]string{"config", "test-notifications"}, d); err != nil {
		t.Fatalf("execute config test-notifications: %v", err)
	}
	if !strings.Contains(stdout.String(), "slack: sent") {
		t.Fatalf("expected slack success line, got: %q", stdout.String())
	}
}

func TestExecute_ConfigTestNotifications_SlackFailureReportsErrorAndPropagates(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		loadConfig: func() (config.Config, error) {
			cfg := config.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}
	err := execute([]string{"config", "test-notifications"}, d)
	if err == nil {
		t.Fatal("expected error when the notification send fails")
	}
	if !strings.Contains(stderr.String(), "slack: failed") {
		t.Fatalf("expected slack failure line on stderr, got: %q", stderr.String())
	}
}

func TestExecute_ConfigTestNotifications_PropagatesLoadConfigError(t *testing.T) {
	t.Parallel()
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Config{}, errors.New("load failed")
		},
	}
	err := execute([]string{"config", "test-notifications"}, d)
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestExecute_Notify_NoneConfiguredPrintsNoticeAndSucceeds(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Default(), nil
		},
	}
	if err := execute([]string{"notify", "hello"}, d); err != nil {
		t.Fatalf("execute notify: %v", err)
	}
	if !strings.Contains(stdout.String(), "no notification service configured") {
		t.Fatalf("expected notice about no configured service, got: %q", stdout.String())
	}
}

func TestExecute_Notify_SlackConfiguredSendsAndReportsSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			cfg := config.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}
	if err := execute([]string{"notify", "hello there"}, d); err != nil {
		t.Fatalf("execute notify: %v", err)
	}
	if !strings.Contains(stdout.String(), "sent to: slack") {
		t.Fatalf("expected slack success line, got: %q", stdout.String())
	}
	if !strings.Contains(gotBody, "hello there") {
		t.Fatalf("expected message text in request body, got: %q", gotBody)
	}
}

func TestExecute_Notify_SlackFailurePropagatesErrorWithoutDoubleReporting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		loadConfig: func() (config.Config, error) {
			cfg := config.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}
	err := execute([]string{"notify", "hello"}, d)
	if err == nil {
		t.Fatal("expected error when the notification send fails")
	}
	if !strings.Contains(err.Error(), "slack") {
		t.Fatalf("expected slack failure mentioned in returned error, got: %v", err)
	}
	// cobra is configured with SilenceErrors, so runNotify must not also print the
	// failure itself - the caller (main) reports the returned error exactly once.
	if stderr.Len() != 0 {
		t.Fatalf("expected runNotify not to write the failure to stderr itself, got: %q", stderr.String())
	}
}

func TestExecute_Notify_PropagatesLoadConfigError(t *testing.T) {
	t.Parallel()
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		loadConfig: func() (config.Config, error) {
			return config.Config{}, errors.New("load failed")
		},
	}
	err := execute([]string{"notify", "hello"}, d)
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestExecute_Notify_RequiresMessageArg(t *testing.T) {
	t.Parallel()
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	if err := execute([]string{"notify"}, d); err == nil {
		t.Fatal("expected error when no message argument is given")
	}
}

func TestRelativeDate_Zero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Time{})
	if got != "unknown time" {
		t.Fatalf("relativeDate(zero) = %q, want %q", got, "unknown time")
	}
}

func TestRelativeDate_NonZero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Now().Add(-time.Hour))
	if got == "" || got == "unknown time" {
		t.Fatalf("relativeDate(now-1h) = %q, expected a non-empty relative string", got)
	}
}

func TestRunEditorCommand_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	err := runEditorCommand("false", "/irrelevant", bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error from false command")
	}
}

func TestRunEditorCommand_MultiWordEditor(t *testing.T) {
	t.Parallel()
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
