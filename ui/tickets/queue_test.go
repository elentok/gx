package tickets

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

func TestQueueModelRendersDependencyAwareEpicPlan(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "04-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-foundation.md"):  true,
		ticketPath(root, "alpha", "02-dependent.md"):   true,
		ticketPath(root, "alpha", "03-independent.md"): true,
		ticketPath(root, "alpha", "04-independent.md"): true,
		ticketPath(root, "beta", "01-other.md"):        true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := m.View().Content

	alpha := strings.Index(content, "alpha")
	parallel := strings.Index(content, "parallel")
	then := strings.Index(content, "then")
	dependent := strings.Index(content, "Dependent")
	beta := strings.Index(content, "beta")
	if alpha < 0 || parallel < alpha || then < parallel || dependent < then || beta < dependent {
		t.Fatalf("expected parallel wave, sequential dependency, and epic grouping in order:\n%s", content)
	}
	if strings.Count(content, "parallel") != 2 {
		t.Fatalf("expected available capacity to produce two parallel clusters, got:\n%s", content)
	}
}

func TestQueueModelSpaceTogglesSharedCheckedSet(t *testing.T) {
	root := t.TempDir()
	name := "01-first.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", name)
	checked := map[string]bool{path: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(spacePress())
	m = updated.(QueueModel)
	if checked[path] {
		t.Fatal("expected Queue uncheck to update the shared checked set")
	}

	updated, _ = m.Update(spacePress())
	if !checked[path] {
		t.Fatal("expected Queue recheck to update the shared checked set")
	}
}

func TestQueueModelIncludesSelectionsAddedAfterLoad(t *testing.T) {
	root := t.TempDir()
	name := "01-later.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", name)
	checked := map[string]bool{}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	checked[path] = true
	if content := m.View().Content; !strings.Contains(content, "Later") {
		t.Fatalf("expected cached Queue model to include a later shared selection:\n%s", content)
	}
}

func loadQueueModel(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	msg := m.Init()()
	updated, _ := m.Update(msg)
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(QueueModel)
}

func ticketPath(root, epic, name string) string {
	return filepath.Join(root, ".scratch", epic, "issues", name)
}
