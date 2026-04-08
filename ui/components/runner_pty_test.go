package components

import (
	"strings"
	"testing"
	"time"
)

func TestCommandRunnerPromptSubmitProvidesInput(t *testing.T) {
	r := NewCommandRunnerWithPolicy(".", "sh", CredentialPolicyPrompt, "-c", "printf \"Enter passphrase for key 'k': \"; read -r pw; if [ \"$pw\" = \"secret\" ]; then printf ok; else printf bad; exit 1; fi")
	r.Start()

	deadline := time.Now().Add(3 * time.Second)
	var prompt CredentialPrompt
	var ok bool
	for time.Now().Before(deadline) {
		prompt, ok = r.Prompt()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatal("expected credential prompt")
	}
	if prompt.Kind != PromptKindSecret {
		t.Fatalf("expected secret prompt, got %v", prompt.Kind)
	}
	if err := r.SubmitPromptInput("secret"); err != nil {
		t.Fatalf("submit prompt input: %v", err)
	}
	if err := r.Wait(); err != nil {
		t.Fatalf("wait: %v\noutput=%q", err, r.Output())
	}
	if !strings.Contains(r.Output(), "ok") {
		t.Fatalf("expected ok in output, got %q", r.Output())
	}
}

func TestCommandRunnerPromptSubmitProvidesInputViaTTY(t *testing.T) {
	r := NewCommandRunnerWithPolicy(".", "sh", CredentialPolicyPrompt, "-c", "printf \"Enter passphrase for key 'k': \" > /dev/tty; IFS= read -r pw < /dev/tty; if [ \"$pw\" = \"secret\" ]; then printf ok > /dev/tty; else printf bad > /dev/tty; exit 1; fi")
	r.Start()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := r.Prompt(); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.SubmitPromptInput("secret"); err != nil {
		t.Fatalf("submit prompt input: %v", err)
	}
	if err := r.Wait(); err != nil {
		t.Fatalf("wait: %v\noutput=%q", err, r.Output())
	}
	if !strings.Contains(r.Output(), "ok") {
		t.Fatalf("expected ok in output, got %q", r.Output())
	}
}

func TestCommandRunnerPromptDoesNotRediscoverStalePromptAfterSubmit(t *testing.T) {
	r := NewCommandRunnerWithPolicy(".", "sh", CredentialPolicyPrompt, "-c", "printf \"Enter passphrase for key 'k': \"; IFS= read -r pw; printf '\\r'; if [ \"$pw\" = \"secret\" ]; then printf ok; else printf bad; exit 1; fi")
	r.Start()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := r.Prompt(); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.SubmitPromptInput("secret"); err != nil {
		t.Fatalf("submit prompt input: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if prompt, ok := r.Prompt(); ok {
		t.Fatalf("unexpected repeated prompt after submit: %+v output=%q", prompt, r.Output())
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("wait: %v\noutput=%q", err, r.Output())
	}
}
