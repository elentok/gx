package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestConfigStatus_ShowsLastReloadAndPendingKey(t *testing.T) {
	dirs := config.DirsFrom(tempEnv(t))
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()

	var out bytes.Buffer
	if err := runServerStatusWith(ctx, dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "config:") {
		t.Errorf("fresh status = %q, want no config block", out.String())
	}

	servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"tcp-listen": true}})
	if _, err := h.Client.ReloadConfig(ctx); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runConfigStatus(ctx, h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var st server.ConfigStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.LastReload == nil || st.LastReload.Outcome != server.EventLevelWarning || !slices.Equal(st.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("config-status --json = %s", out.String())
	}

	out.Reset()
	if err := runServerStatusWith(ctx, dirs, h.Client, true, &out); err != nil {
		t.Fatal(err)
	}
	var info StatusInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Config == nil || info.Config.LastReload == nil || !slices.Equal(info.Config.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("status --json = %s, want the config block", out.String())
	}

	out.Reset()
	if err := runServerStatusWith(ctx, dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"config:\n  last reload: warning (command, ", "\n  restart needed for: server.tcp-listen\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status = %q, want %q", out.String(), want)
		}
	}
}

func TestServerStatus_BrokenConfigStillReportsTheError(t *testing.T) {
	env := tempEnv(t)
	home := envValue(env, "HOME")
	if err := os.MkdirAll(filepath.Join(home, ".config", "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "gx", "config.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.Start(t)

	var out bytes.Buffer
	if err := runServerStatusWith(context.Background(), config.DirsFrom(env), h.Client, false, &out); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.HasPrefix(out.String(), "config error: config ") || !strings.Contains(out.String(), "\nrunning\n") {
		t.Errorf("status = %q, want the config error on top", out.String())
	}
}

func TestServerStatus_PrintsFingerprintAndAWarningPerLeftOutHost(t *testing.T) {
	dirs := config.DirsFrom(tempEnv(t))
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = []string{"a.example.org"}
	})
	servertest.WriteConfig(t, path, store, map[string]any{
		"server": map[string]any{"web": map[string]any{"allowed-hosts": []string{"a.example.org", "outside.example.net"}}},
	})
	if res, err := h.Client.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload: %+v, %v", res, err)
	}
	// Status reads the token from the user's state dir, which the harness does not use.
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	stateDir, err := dirs.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(server.WebTokenPath(stateDir), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runServerStatusWith(context.Background(), dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^fingerprint: ([0-9a-f]{8} ){7}[0-9a-f]{8}$`).MatchString(out.String()) {
		t.Errorf("status = %q, want a fingerprint line in 8 groups of 8", out.String())
	}
	if !strings.Contains(out.String(), "warning: web certificate leaves out allowed host outside.example.net") {
		t.Errorf("status = %q, want a warning for outside.example.net", out.String())
	}
	if strings.Contains(out.String(), "allowed host a.example.org") {
		t.Errorf("status = %q, want no warning for the permitted host", out.String())
	}
}

func TestServerStatus_PrintsThePlainWebURLAndTheSetupCodeWhilePending(t *testing.T) {
	dirs := config.DirsFrom(tempEnv(t))
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	// web_url in --json still carries the token and reads it from the user's state dir, which the harness does not use.
	stateDir, err := dirs.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(server.WebTokenPath(stateDir), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info, err := h.Client.WebInfo(context.Background())
	if err != nil || info.SetupCode == "" {
		t.Fatalf("web info %+v, %v", info, err)
	}

	var out bytes.Buffer
	if err := runServerStatusWith(context.Background(), dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	want := "web: " + hs.WebURL + "\nsetup code: " + info.SetupCode + " (expires "
	if !strings.Contains(out.String(), want) {
		t.Errorf("status = %q, want it to contain %q", out.String(), want)
	}
	if strings.Contains(out.String(), "gx_token") || strings.Contains(out.String(), token) {
		t.Errorf("status = %q, want no token in the text output", out.String())
	}

	var js bytes.Buffer
	if err := runServerStatusWith(context.Background(), dirs, h.Client, true, &js); err != nil {
		t.Fatal(err)
	}
	var got StatusInfo
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, js.String())
	}
	if got.WebSetupCode != info.SetupCode || got.WebSetupExpires.IsZero() {
		t.Errorf("json setup code %q expires %v, want %q with an expiry", got.WebSetupCode, got.WebSetupExpires, info.SetupCode)
	}
}

func TestServerStatus_NoSetupCodeAfterSetup(t *testing.T) {
	dirs := config.DirsFrom(tempEnv(t))
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	stateDir, err := dirs.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(server.WebTokenPath(stateDir), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	finishWebSetup(t, h)

	var out, js bytes.Buffer
	if err := runServerStatusWith(context.Background(), dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if err := runServerStatusWith(context.Background(), dirs, h.Client, true, &js); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "setup code") || strings.Contains(js.String(), "web_setup_code") {
		t.Errorf("status holds a setup code after setup:\n%s\n%s", out.String(), js.String())
	}
}
