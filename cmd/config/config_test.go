package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	gxconfig "github.com/elentok/gx/config"
)

// run executes args under a mini root holding `config` and `notify`.
func run(t *testing.T, d Deps, args ...string) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, args, New(d), NewNotify(d))
}

func TestExecute_ConfigEdit_RequiresEditor(t *testing.T) {
	t.Parallel()
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdout: bytes.NewBuffer(nil),
			Stderr: bytes.NewBuffer(nil),
			Getenv: func(string) string { return "" },
		},
		InitConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
	}

	err := run(t, d, "config", "edit")
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
	d := Deps{
		Env: cmdbase.Env{
			Env:    cmdtest.EnvFor(t),
			Stdout: &stdout,
			Stderr: bytes.NewBuffer(nil),
			Getenv: func(k string) string {
				if k == "EDITOR" {
					return "vim"
				}
				return ""
			},
		},
		InitConfig: func() (string, error) {
			return "/tmp/gx/config.json", nil
		},
		RunEditor: func(editor, path string, _ io.Reader, _, _ io.Writer) error {
			gotEditor = editor
			gotPath = path
			return nil
		},
	}

	if err := run(t, d, "config", "edit"); err != nil {
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
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
	}}
	if err := run(t, d, "config", "defaults"); err != nil {
		t.Fatalf("execute config defaults: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "use-nerdfont-icons") {
		t.Fatalf("expected config key in output, got: %q", out)
	}
	if !strings.Contains(out, "max-agents-per-epic") {
		t.Fatalf("expected execution queue config keys in output, got: %q", out)
	}
	if strings.Contains(out, "max-concurrent-tickets-per-epic") || strings.Contains(out, "max-concurrent-epics") {
		t.Fatalf("old cap keys must not be printed, got: %q", out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
}

func TestExecute_ConfigShow_PrintsJSON(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Default(), nil
		},
	}}
	if err := run(t, d, "config", "show"); err != nil {
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
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Config{}, errors.New("load failed")
		},
	}}
	err := run(t, d, "config", "show")
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestExecute_ConfigTestNotifications_NoneConfiguredPrintsNoticeAndSucceeds(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Default(), nil
		},
	}}
	if err := run(t, d, "config", "test-notifications"); err != nil {
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
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			cfg := gxconfig.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}}
	if err := run(t, d, "config", "test-notifications"); err != nil {
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
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: &stderr,
		LoadConfig: func() (gxconfig.Config, error) {
			cfg := gxconfig.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}}
	err := run(t, d, "config", "test-notifications")
	if err == nil {
		t.Fatal("expected error when the notification send fails")
	}
	if !strings.Contains(stderr.String(), "slack: failed") {
		t.Fatalf("expected slack failure line on stderr, got: %q", stderr.String())
	}
}

func TestExecute_ConfigTestNotifications_PropagatesLoadConfigError(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Config{}, errors.New("load failed")
		},
	}}
	err := run(t, d, "config", "test-notifications")
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestExecute_Notify_NoneConfiguredPrintsNoticeAndSucceeds(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Default(), nil
		},
	}}
	if err := run(t, d, "notify", "hello"); err != nil {
		t.Fatalf("execute notify: %v", err)
	}
	if !strings.Contains(stdout.String(), "no notification service configured") {
		t.Fatalf("expected notice about no configured service, got: %q", stdout.String())
	}
}

func TestExecute_Notify_SlackConfiguredSendsAndReportsSuccess(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			cfg := gxconfig.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}}
	if err := run(t, d, "notify", "hello there"); err != nil {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: &stdout,
		Stderr: &stderr,
		LoadConfig: func() (gxconfig.Config, error) {
			cfg := gxconfig.Default()
			cfg.Notifications.Slack.WebhookURL = server.URL
			return cfg, nil
		},
	}}
	err := run(t, d, "notify", "hello")
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
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		LoadConfig: func() (gxconfig.Config, error) {
			return gxconfig.Config{}, errors.New("load failed")
		},
	}}
	err := run(t, d, "notify", "hello")
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got: %v", err)
	}
}

func TestExecute_Notify_RequiresMessageArg(t *testing.T) {
	t.Parallel()
	d := Deps{Env: cmdbase.Env{
		Env:    cmdtest.EnvFor(t),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
	}}
	if err := run(t, d, "notify"); err == nil {
		t.Fatal("expected error when no message argument is given")
	}
}
