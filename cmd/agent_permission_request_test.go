package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
)

const permissionPayload = `{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rg foo"}}`

// runPermissionHook runs the hook on stdin. A non-nil answer is called with the
// request the hook wrote and its result is written as the decision; with nil
// the wait is cancelled instead.
func runPermissionHook(t *testing.T, dir, stdin string, answer func(nativerunner.PermissionRequest)) string {
	t.Helper()
	c := newAgentPermissionRequestCmd()
	var stdout bytes.Buffer
	c.SetArgs([]string{"--agent-dir", dir, "--poll", "5ms"})
	c.SetIn(strings.NewReader(stdin))
	c.SetOut(&stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.ExecuteContext(ctx) }()
	if answer == nil {
		time.Sleep(50 * time.Millisecond)
		cancel()
	} else {
		for {
			req, ok, err := nativerunner.ReadPermissionRequest(dir)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				answer(req)
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return stdout.String()
}

func TestAgentPermissionRequest_AllowAndDeny(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		allow bool
		want  string
	}{
		{"allow", true, `{"hookSpecificOutput":{"decision":{"behavior":"allow"},"hookEventName":"PermissionRequest"}}`},
		{"deny", false, `{"hookSpecificOutput":{"decision":{"behavior":"deny","message":"use fd instead"},"hookEventName":"PermissionRequest"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var seen nativerunner.PermissionRequest
			out := runPermissionHook(t, dir, permissionPayload, func(req nativerunner.PermissionRequest) {
				seen = req
				d := nativerunner.PermissionDecision{ID: req.ID, Allow: tc.allow}
				if !tc.allow {
					d.Reason = "use fd instead"
				}
				if err := nativerunner.WritePermissionDecision(dir, d); err != nil {
					t.Error(err)
				}
			})
			if seen.ID == "" || seen.Tool != "Bash" || seen.Description != "rg foo" || string(seen.Input) != `{"command":"rg foo"}` {
				t.Errorf("request = %+v", seen)
			}
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("output = %s, want %s", out, tc.want)
			}
			if _, ok, _ := nativerunner.ReadPermissionRequest(dir); ok {
				t.Error("request file left behind")
			}
		})
	}
}

func TestAgentPermissionRequest_IgnoresADecisionForAnotherRequest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := runPermissionHook(t, dir, permissionPayload, func(req nativerunner.PermissionRequest) {
		// A stale decision first, then the right one.
		_ = nativerunner.WritePermissionDecision(dir, nativerunner.PermissionDecision{ID: "stale"})
		time.Sleep(30 * time.Millisecond)
		_ = nativerunner.WritePermissionDecision(dir, nativerunner.PermissionDecision{ID: req.ID, Allow: true})
	})
	if !strings.Contains(out, `"allow"`) {
		t.Errorf("output = %s, want an allow", out)
	}
}

func TestAgentPermissionRequest_NoDecisionWhenThereIsNothingToAnswerTo(t *testing.T) {
	t.Parallel()
	t.Run("missing agent dir", func(t *testing.T) {
		t.Parallel()
		if out := runPermissionHook(t, filepath.Join(t.TempDir(), "gone"), permissionPayload, nil); out != "" {
			t.Errorf("output = %q, want none", out)
		}
	})
	t.Run("cancelled wait", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if out := runPermissionHook(t, dir, permissionPayload, nil); out != "" {
			t.Errorf("output = %q, want none", out)
		}
		if _, err := os.Stat(filepath.Join(dir, nativerunner.PermissionRequestFile)); err == nil {
			t.Error("request file left behind after the cancel")
		}
	})
	t.Run("unreadable payload", func(t *testing.T) {
		t.Parallel()
		if out := runPermissionHook(t, t.TempDir(), "not json", nil); out != "" {
			t.Errorf("output = %q, want none", out)
		}
	})
}
