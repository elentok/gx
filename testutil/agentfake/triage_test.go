package agentfake

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
