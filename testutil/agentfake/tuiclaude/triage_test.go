package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassify_TriagePromptPlaysTriageAndAWorkerPromptIsUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, WorkerConfigFile), []byte(`{"triage":{"verify_batch":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var tu tui
	tu.pid.Cwd = dir
	if kind, _, _ := tu.classify(TriagePromptPrefix + "--project p"); kind != promptTriage {
		t.Errorf("triage prompt kind = %v, want promptTriage", kind)
	}
	if kind, address, _ := tu.classify("/gx-implement p:epic/01"); kind != promptWorker || address != "p:epic/01" {
		t.Errorf("worker prompt = %v %q, want promptWorker p:epic/01", kind, address)
	}
}

func TestClassify_ATypedMessageOfATriageSessionGoesToThePlayer(t *testing.T) {
	t.Parallel()
	var tu tui
	if kind, _, _ := tu.classify("yes"); kind == promptTriage {
		t.Errorf("typed message without a Triage session = %v, want it left alone", kind)
	}
	tu.triageProject = "p"
	if kind, _, _ := tu.classify("yes"); kind != promptTriage {
		t.Errorf("typed message of a Triage session = %v, want promptTriage", kind)
	}
	if kind, _, _ := tu.classify("/compact"); kind == promptTriage {
		t.Errorf("slash command of a Triage session = %v, want it left alone", kind)
	}
}

func TestClassify_CompactWithArgumentsIsAPlainPromptEvenWithAWorkerConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, WorkerConfigFile), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tu := &tui{pid: PidFile{Cwd: dir}}
	for _, text := range []string{"/compact", "/compact foo"} {
		if kind, _, _ := tu.classify(text); kind != promptPlain {
			t.Errorf("classify(%q) = %v, want promptPlain", text, kind)
		}
	}
}

func TestSkillPromptKnobs_IgnoresCompactWithArguments(t *testing.T) {
	t.Parallel()
	tu := &tui{opts: tuiOptions{noTurn: true}}
	for _, text := range []string{"/compact", "/compact foo"} {
		if handled, err := tu.skillPromptKnobs(text); handled || err != nil {
			t.Errorf("skillPromptKnobs(%q) = %v, %v, want false, nil", text, handled, err)
		}
	}
}
