package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintBadge_NonTerminal(t *testing.T) {
	var buf bytes.Buffer
	printBadge(&buf, false, "nerd text", "plain text")
	got := buf.String()
	if !strings.Contains(got, "plain text") {
		t.Fatalf("printBadge output = %q, want to contain %q", got, "plain text")
	}
}

func TestPrintBadge_NonTerminal_Nerd(t *testing.T) {
	var buf bytes.Buffer
	printBadge(&buf, true, "nerd text", "plain text")
	got := buf.String()
	if !strings.Contains(got, "nerd text") {
		t.Fatalf("printBadge output = %q, want to contain %q", got, "nerd text")
	}
}

func TestPrintSuccess_NonTerminal(t *testing.T) {
	var buf bytes.Buffer
	printSuccess(&buf, "it worked")
	got := buf.String()
	if !strings.Contains(got, "✔") {
		t.Fatalf("printSuccess output = %q, want to contain ✔", got)
	}
	if !strings.Contains(got, "it worked") {
		t.Fatalf("printSuccess output = %q, want to contain %q", got, "it worked")
	}
}

func TestPrintError_NonTerminal(t *testing.T) {
	var buf bytes.Buffer
	printError(&buf, "it failed")
	got := buf.String()
	if !strings.Contains(got, "✘") {
		t.Fatalf("printError output = %q, want to contain ✘", got)
	}
	if !strings.Contains(got, "it failed") {
		t.Fatalf("printError output = %q, want to contain %q", got, "it failed")
	}
}
