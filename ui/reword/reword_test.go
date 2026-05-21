package reword

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/components"
)

func TestNew(t *testing.T) {
	m := New()
	if m.Hash != "" || m.Subject != "" {
		t.Error("expected empty hash/subject on New()")
	}
}

func TestHasFailed_Empty(t *testing.T) {
	m := New()
	if m.hasFailed() {
		t.Error("expected hasFailed=false with no steps")
	}
}

func TestHasFailed_WithFailed(t *testing.T) {
	m := New()
	m.steps = []execStep{
		{Step: components.Step{TitleFailed: "reword failed", HasFailed: true}},
	}
	if !m.hasFailed() {
		t.Error("expected hasFailed=true when step has failed")
	}
}

func TestStepErr_NoFailure(t *testing.T) {
	m := New()
	if m.stepErr() != nil {
		t.Error("expected nil stepErr with no failed steps")
	}
}

func TestStepErr_WithFailure(t *testing.T) {
	m := New()
	m.steps = []execStep{
		{Step: components.Step{TitleFailed: "reword failed", HasFailed: true}},
	}
	err := m.stepErr()
	if err == nil {
		t.Fatal("expected non-nil stepErr")
	}
	if err.Error() != "reword failed" {
		t.Errorf("stepErr() = %q, want 'reword failed'", err.Error())
	}
}

func TestStepError_Error(t *testing.T) {
	e := &StepError{Title: "step error"}
	if e.Error() != "step error" {
		t.Errorf("StepError.Error() = %q, want 'step error'", e.Error())
	}
}

func TestView_Basic(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.Hash = "abc1234567"
	m.Subject = "my commit"
	view := m.View(80)
	if view == "" {
		t.Error("expected non-empty View")
	}
}

func TestUpdate_EscCloses_WhenFailed(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.steps = []execStep{
		{Step: components.Step{TitleFailed: "step failed", HasFailed: true}},
	}
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsOpen {
		t.Error("expected IsOpen=false after esc when failed")
	}
	if !result.Done {
		t.Error("expected Done=true after esc when failed")
	}
}

func TestUpdate_EscNoopWhenNotFailed(t *testing.T) {
	m := New()
	m.IsOpen = true
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !next.IsOpen {
		t.Error("expected IsOpen=true: esc is a no-op when not failed")
	}
	if result.Done {
		t.Error("expected Done=false when not failed")
	}
}

func TestUpdate_NonKeyMsg(t *testing.T) {
	m := New()
	m.IsOpen = true
	_, _, result := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if result.Done {
		t.Error("window size msg should not trigger any result")
	}
}

func TestCmdOpenEditor_NoEnv(t *testing.T) {
	// Ensure EDITOR is unset for this test
	t.Setenv("EDITOR", "")
	m := New()
	_, err := m.CmdOpenEditor("/tmp", "abc123", "subject", "body", false)
	if err == nil {
		t.Error("expected error when EDITOR is not set")
	}
}

func TestCmdOpenEditor_WithEditor(t *testing.T) {
	t.Setenv("EDITOR", "true") // "true" is a valid no-op command
	m := New()
	cmd, err := m.CmdOpenEditor("/tmp", "abc123", "subject", "body", false)
	if err != nil {
		t.Fatalf("CmdOpenEditor: %v", err)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
	if m.Hash != "abc123" {
		t.Errorf("Hash = %q, want 'abc123'", m.Hash)
	}
	if m.tmpFile == "" {
		t.Error("expected tmpFile to be set")
	}
	// Clean up temp file
	if m.tmpFile != "" {
		os.Remove(m.tmpFile)
	}
}

func TestReadEditorResult_Unchanged(t *testing.T) {
	original := "my subject"
	f, err := os.CreateTemp("", "reword-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(original + "\n")
	f.Close()

	m := New()
	m.origMsg = original
	m.tmpFile = f.Name()

	changed, _, err := m.ReadEditorResult()
	if err != nil {
		t.Fatalf("ReadEditorResult: %v", err)
	}
	if changed {
		t.Error("expected changed=false for unchanged message")
	}
}

func TestReadEditorResult_Changed(t *testing.T) {
	f, err := os.CreateTemp("", "reword-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("new subject\n")
	f.Close()

	m := New()
	m.origMsg = "original subject"
	m.tmpFile = f.Name()

	changed, newMsg, err := m.ReadEditorResult()
	if err != nil {
		t.Fatalf("ReadEditorResult: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for modified message")
	}
	if !strings.HasPrefix(newMsg, "new subject") {
		t.Errorf("newMsg = %q, expected to start with 'new subject'", newMsg)
	}
}
