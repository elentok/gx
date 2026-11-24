package config

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	gxconfig "github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// slackHook counts the posts a Slack webhook gets.
func slackHook(t *testing.T) (url string, posts *atomic.Int32) {
	t.Helper()
	posts = new(atomic.Int32)
	hook := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { posts.Add(1) }))
	t.Cleanup(hook.Close)
	return hook.URL, posts
}

// routerDeps is Deps whose config is the defaults plus tweak. newClient nil means no server.
func routerDeps(t *testing.T, newClient func() (*apiclient.Client, error), tweak func(*gxconfig.Config)) (Deps, *bytes.Buffer) {
	t.Helper()
	var stdout bytes.Buffer
	return Deps{Env: cmdbase.Env{
		Env:       cmdtest.EnvFor(t),
		Stdout:    &stdout,
		Stderr:    bytes.NewBuffer(nil),
		NewClient: newClient,
		LoadConfig: func() (gxconfig.Config, error) {
			cfg := gxconfig.Default()
			tweak(&cfg)
			return cfg, nil
		},
	}}, &stdout
}

// oldServer answers every request 404, as a server built before the routes does.
func oldServer(t *testing.T) func() (*apiclient.Client, error) {
	t.Helper()
	sock := filepath.Join(cmdtest.ShortTempDir(t), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler()}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return func() (*apiclient.Client, error) { return apiclient.New(sock), nil }
}

func TestExecute_Notify_ServerUpSendsToCheckedChannelsOnly(t *testing.T) {
	opt, count := rigs.ChatRecorder(t)
	h := servertest.Start(t, opt)
	d, stdout := routerDeps(t, func() (*apiclient.Client, error) { return h.Client, nil }, func(*gxconfig.Config) {})

	if err := run(t, d, "notify", "hello router"); err != nil {
		t.Fatalf("execute notify: %v", err)
	}

	if n := rigs.WaitForChat(count, "hello router"); n != 1 {
		t.Errorf("slack got the message %d times, want 1", n)
	}
	out := stdout.String()
	if !strings.Contains(out, "sent to: ") || !strings.Contains(strings.SplitN(out, "\n", 2)[0], "slack") {
		t.Errorf("output does not name slack as sent to: %q", out)
	}
	// The harness's Desktop list lacks the CLI message kind.
	if !strings.Contains(out, "desktop (unchecked)") {
		t.Errorf("output does not say Desktop was unchecked: %q", out)
	}
}

func TestExecute_Notify_NoServerFallsBackToTelegramAndSlack(t *testing.T) {
	url, posts := slackHook(t)
	tests := []struct {
		name      string
		tweak     func(*gxconfig.Config)
		wantPosts int32
		wantOut   string
	}{
		{"checked", func(c *gxconfig.Config) {}, 1, "sent to: slack"},
		{"unchecked", func(c *gxconfig.Config) {
			c.NotificationChannels.Slack.Kinds = slices.DeleteFunc(slices.Clone(c.NotificationChannels.Slack.Kinds), func(k string) bool { return k == notifykind.CLIMessage })
		}, 0, "slack (unchecked)"},
		{"off", func(c *gxconfig.Config) { c.NotificationChannels.Slack.Enabled = false }, 0, "slack (off)"},
	}
	for _, tc := range tests {
		posts.Store(0)
		d, stdout := routerDeps(t, nil, func(c *gxconfig.Config) {
			c.Notifications.Slack.WebhookURL = url
			tc.tweak(c)
		})
		if err := run(t, d, "notify", "hello"); err != nil {
			t.Fatalf("%s: execute notify: %v", tc.name, err)
		}
		if got := posts.Load(); got != tc.wantPosts {
			t.Errorf("%s: slack posts = %d, want %d", tc.name, got, tc.wantPosts)
		}
		out := stdout.String()
		if !strings.Contains(out, tc.wantOut) || !strings.Contains(out, "desktop (skipped), web-toast (skipped)") || !strings.Contains(out, "push (skipped)") {
			t.Errorf("%s: output = %q, want %q and the other three skipped", tc.name, out, tc.wantOut)
		}
	}
}

func TestExecute_Notify_OldServer404FallsBack(t *testing.T) {
	url, posts := slackHook(t)
	d, stdout := routerDeps(t, oldServer(t), func(c *gxconfig.Config) { c.Notifications.Slack.WebhookURL = url })

	if err := run(t, d, "notify", "hello old server"); err != nil {
		t.Fatalf("execute notify: %v", err)
	}

	if posts.Load() != 1 || !strings.Contains(stdout.String(), "sent to: slack") {
		t.Errorf("posts = %d, output = %q, want one direct slack send", posts.Load(), stdout.String())
	}
}

func TestExecute_ConfigTestNotifications_ServerUpSkipsAnOffChannelAndSaysSo(t *testing.T) {
	opt, count := rigs.ChatRecorder(t)
	h := servertest.Start(t, opt, func(c *server.Config) { c.NotifyChannels.Slack.Off = true })
	d, stdout := routerDeps(t, func() (*apiclient.Client, error) { return h.Client, nil }, func(*gxconfig.Config) {})

	if err := run(t, d, "config", "test-notifications"); err != nil {
		t.Fatalf("execute config test-notifications: %v", err)
	}

	if n := count("Test notification"); n != 0 {
		t.Errorf("slack got %d test messages with its switch off", n)
	}
	if !strings.Contains(stdout.String(), "slack: off") {
		t.Errorf("output does not say slack is off: %q", stdout.String())
	}
}

func TestExecute_ConfigTestNotifications_NoServerSkipsTheOtherThree(t *testing.T) {
	url, posts := slackHook(t)
	d, stdout := routerDeps(t, nil, func(c *gxconfig.Config) { c.Notifications.Slack.WebhookURL = url })

	if err := run(t, d, "config", "test-notifications"); err != nil {
		t.Fatalf("execute config test-notifications: %v", err)
	}

	out := stdout.String()
	if posts.Load() != 1 || !strings.Contains(out, "slack: sent") {
		t.Errorf("posts = %d, output = %q, want one slack test", posts.Load(), out)
	}
	for _, ch := range []string{"desktop", "web-toast", "push"} {
		if !strings.Contains(out, ch+": skipped") {
			t.Errorf("output does not say %s was skipped: %q", ch, out)
		}
	}
}

func TestExecute_ConfigTestNotifications_NoServerSkipsAnOffChannel(t *testing.T) {
	url, posts := slackHook(t)
	d, stdout := routerDeps(t, nil, func(c *gxconfig.Config) {
		c.Notifications.Slack.WebhookURL = url
		c.NotificationChannels.Slack.Enabled = false
	})

	if err := run(t, d, "config", "test-notifications"); err != nil {
		t.Fatalf("execute config test-notifications: %v", err)
	}

	if posts.Load() != 0 || !strings.Contains(stdout.String(), "slack: off") {
		t.Errorf("posts = %d, output = %q, want no send and slack off", posts.Load(), stdout.String())
	}
}

func TestExecute_Notify_JSONOnlyWithAMessage(t *testing.T) {
	url, _ := slackHook(t)
	d, stdout := routerDeps(t, nil, func(c *gxconfig.Config) { c.Notifications.Slack.WebhookURL = url })

	if err := run(t, d, "notify", "--json", "--status"); err == nil || !strings.Contains(err.Error(), "--json") {
		t.Errorf("--json with --status = %v, want a --json refusal", err)
	}
	if err := run(t, d, "notify", "--status", "--enable", "slack"); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("--status with --enable = %v, want mutually exclusive", err)
	}

	if err := run(t, d, "notify", "--json", "hi"); err != nil {
		t.Fatalf("notify --json hi: %v", err)
	}
	var got struct {
		Via      string `json:"via"`
		Channels []struct{ Channel, Outcome string }
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("--json output is not JSON: %v: %q", err, stdout.String())
	}
	if got.Via != cmdbase.ViaDirect || len(got.Channels) != 5 {
		t.Errorf("json = %+v, want via direct and five channels", got)
	}
}
