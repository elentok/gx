package commit

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
)

func TestNewLoadsCommitDetails(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "first")

	m := New(repo, "HEAD")
	if m.err != nil {
		t.Fatalf("New err: %v", m.err)
	}
	if m.details.Hash == "" || m.details.Subject != "first" {
		t.Fatalf("unexpected details: %#v", m.details)
	}
}

func TestBToggleBody(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nbody")

	m := New(repo, "HEAD")
	if !m.bodyExpanded {
		t.Fatalf("expected body expanded by default")
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if m.bodyExpanded {
		t.Fatalf("expected body collapsed after b")
	}
	m.ready = true
	m.width = 80
	m.height = 20
	if !strings.Contains(m.View().Content, "body hidden") {
		t.Fatalf("expected collapsed body hint")
	}
}
