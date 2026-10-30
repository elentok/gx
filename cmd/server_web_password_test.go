package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/termios"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

const newWebPassword = "a brand new passphrase"

// webLoginReason posts a login and returns the refusal reason ("" when it is not refused).
func webLoginReason(t *testing.T, baseURL, password, secret string) string {
	t.Helper()
	code, err := server.TOTPCode(secret, time.Now().Unix()/30+1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"password": password, "totp_code": code})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/web/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Transport: servertest.WebTransport()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var res struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res.Reason
}

func TestWebPassword_StdinSignsEveryLoginOutAndTheNewPasswordLogsIn(t *testing.T) {
	h := startWebServer(t)
	old, secret := finishWebSetupWithSecret(t, h)
	base := mustWebInfo(t, h).BaseURL

	var out bytes.Buffer
	args := []string{"server", "web-password", "--password-stdin", "--json"}
	if err := execute(args, webResetDeps(t, h, newWebPassword+"\n", false, &out)); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if list, _ := h.Client.WebLogins(context.Background()); len(list.Logins) != 0 {
		t.Errorf("logins after the change = %+v, want none", list.Logins)
	}
	if got := webLoginsStatus(t, base, old); got != http.StatusUnauthorized {
		t.Errorf("old cookie after the change = %d, want 401", got)
	}
	if got := webLoginReason(t, base, "correct horse battery", secret); got != server.ReasonWrongCredentials {
		t.Errorf("login with the old password = %q, want %s", got, server.ReasonWrongCredentials)
	}
	if got := webLoginReason(t, base, newWebPassword, secret); got != "" {
		t.Errorf("login with the new password was refused: %q", got)
	}
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	if !strings.Contains(string(logs), `"msg":"web password changed","ip":"cli"`) {
		t.Errorf("gx.log has no password change line for ip cli:\n%s", logs)
	}
}

func TestWebPassword_StdinEndsTheOpenEventStream(t *testing.T) {
	h := startWebServer(t)
	cookie := finishWebSetup(t, h)
	req, _ := http.NewRequest(http.MethodGet, mustWebInfo(t, h).BaseURL+"/v1/events", nil)
	req.AddCookie(cookie)
	stream, err := (&http.Client{Transport: servertest.WebTransport()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(ended) }()

	if err := execute([]string{"server", "web-password", "--password-stdin"}, webResetDeps(t, h, newWebPassword+"\n", false, &bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("the event stream stayed open after the password change")
	}
}

func TestWebPassword_ShortPasswordIsRefusedWithItsReason(t *testing.T) {
	h := startWebServer(t)
	old := finishWebSetup(t, h)
	var out bytes.Buffer
	err := execute([]string{"server", "web-password", "--password-stdin", "--json"}, webResetDeps(t, h, "short\n", false, &out))
	if err == nil || !strings.Contains(err.Error(), server.ReasonPasswordTooShort) {
		t.Errorf("err = %v, want %s", err, server.ReasonPasswordTooShort)
	}
	if !strings.Contains(out.String(), server.ReasonPasswordTooShort) {
		t.Errorf("--json output has no reason:\n%s", out.String())
	}
	if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, old); got != http.StatusOK {
		t.Errorf("old cookie after a refused change = %d, want 200", got)
	}
}

func TestWebPassword_NoTerminalAndNoStdinFlagRefuses(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)
	err := execute([]string{"server", "web-password"}, webResetDeps(t, h, newWebPassword+"\n", false, &bytes.Buffer{}))
	if err == nil || !strings.Contains(err.Error(), "--password-stdin") {
		t.Errorf("err = %v, want a refusal naming --password-stdin", err)
	}
}

// readUntil reads the pty until it has seen want.
func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	var seen strings.Builder
	buf := make([]byte, 256)
	for !strings.Contains(seen.String(), want) {
		n, err := r.Read(buf)
		seen.Write(buf[:n])
		if err != nil {
			t.Fatalf("pty closed before %q; saw %q", want, seen.String())
		}
	}
	return seen.String()
}

// waitForNoEcho blocks until the command has switched the terminal's echo off,
// so typed input is never echoed by a race with the prompt.
func waitForNoEcho(t *testing.T, tty *os.File) {
	t.Helper()
	testutil.WaitFor(t, 5*time.Second, "the command to switch the terminal's echo off", func() bool {
		tio, err := termios.GetTermios(int(tty.Fd()))
		return err == nil && tio.Lflag&unix.ECHO == 0
	})
}

// runOnPty runs web-password with its stdin and stdout on a pty, types each
// answer once its prompt shows, and returns everything the terminal displayed.
func runOnPty(t *testing.T, h *servertest.Harness, answers ...string) (string, error) {
	t.Helper()
	master, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty here: %v", err)
	}
	defer master.Close()
	defer tty.Close()
	d := webResetDeps(t, h, "", true, &bytes.Buffer{})
	d.stdin, d.stdout = tty, tty

	done := make(chan error, 1)
	go func() { done <- execute([]string{"server", "web-password"}, d) }()
	var screen strings.Builder
	for i, answer := range answers {
		prompt := "New web password: "
		if i > 0 {
			prompt = "again: "
		}
		screen.WriteString(readUntil(t, master, prompt))
		waitForNoEcho(t, tty)
		if _, err := master.WriteString(answer + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-done:
		rest := make([]byte, 4096)
		master.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _ := master.Read(rest)
		screen.Write(rest[:n])
		return screen.String(), err
	case <-time.After(10 * time.Second):
		t.Fatal("web-password did not finish")
		return "", nil
	}
}

func TestWebPassword_AsksTwiceOnATerminalWithoutEcho(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)
	screen, err := runOnPty(t, h, newWebPassword, newWebPassword)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(screen, newWebPassword) {
		t.Errorf("the terminal echoed the password:\n%q", screen)
	}
	if list, _ := h.Client.WebLogins(context.Background()); len(list.Logins) != 0 {
		t.Errorf("logins after the change = %+v, want none", list.Logins)
	}
}

func TestWebPassword_TerminalMismatchChangesNothing(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)
	_, err := runOnPty(t, h, newWebPassword, "another different one")
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Errorf("err = %v, want a mismatch refusal", err)
	}
	if list, _ := h.Client.WebLogins(context.Background()); len(list.Logins) != 1 {
		t.Errorf("logins after a mismatch = %+v, want the one login kept", list.Logins)
	}
}

func TestWebPassword_NoServerIsServerNotRunning(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gxn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	c := apiclient.New(filepath.Join(dir, "none.sock"))
	err = runWebPassword(context.Background(), c, newWebPassword, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, server.ReasonServerNotRunning)
	}
}

func TestWebPassword_RefusesWhenTheWebUIIsOff(t *testing.T) {
	h := servertest.Start(t)
	err := runWebPassword(context.Background(), h.Client, newWebPassword, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "the web UI is off") {
		t.Errorf("err = %v, want the web UI is off", err)
	}
}
