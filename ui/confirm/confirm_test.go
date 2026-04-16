package confirm

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunYes(t *testing.T) {
	ok, err := run("Force push?", false, strings.NewReader("y"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !ok {
		t.Fatal("expected yes")
	}
}

func TestRunNo(t *testing.T) {
	ok, err := run("Force push?", false, strings.NewReader("n"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if ok {
		t.Fatal("expected no")
	}
}

func TestRunDefaultYes(t *testing.T) {
	ok, err := run("Force push?", false, strings.NewReader("\r"), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !ok {
		t.Fatal("expected yes as default")
	}
}
