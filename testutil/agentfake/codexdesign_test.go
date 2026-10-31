package agentfake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// assistantTexts are the assistant messages of a rollout, in order.
func assistantTexts(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var l struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("bad rollout line %q: %v", line, err)
		}
		if l.Type == "response_item" && l.Payload.Type == "message" && l.Payload.Role == "assistant" {
			texts = append(texts, l.Payload.Content[0].Text)
		}
	}
	return texts
}

func TestCodexDesign_PlaysTheScriptedTurnsAsAssistantMessages(t *testing.T) {
	repo := t.TempDir()
	raw, err := json.Marshal(WorkerConfig{Design: []string{DesignTurnRound, DesignTurnRunEnd}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, WorkerConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	p := startCodexIn(t, repo, home, "design1")
	p.startTurn("$gx-design demo:map")
	p.waitOut("state=done")
	p.startTurn("an answer")
	// The output already holds the first turn's states, so wait on the rollout.
	var texts []string
	for deadline := time.Now().Add(10 * time.Second); len(texts) < 2 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		texts = assistantTexts(t, rolloutFiles(t, home)[0])
	}
	if len(texts) != 2 {
		t.Fatalf("assistant messages = %q, want two", texts)
	}
	if !strings.Contains(texts[0], "Q8") {
		t.Errorf("first message = %q, want the first round", texts[0])
	}
	if !strings.HasSuffix(texts[1], DesignRunEnd) {
		t.Errorf("second message = %q, want the run-end message", texts[1])
	}
}
