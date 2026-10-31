package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTurnModel_NewestTurnContextWins(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	body := `{"type":"session_meta","payload":{"id":"s"}}
{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"low"}}
not json
{"type":"turn_context","payload":{"model":"gpt-6.1-terra","effort":"high"}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, ok, err := ReadTurnModel(path)
	if err != nil || !ok || m != (TurnModel{Model: "gpt-6.1-terra", Effort: "high"}) {
		t.Errorf("got %+v ok=%v err=%v, want gpt-6.1-terra/high", m, ok, err)
	}
}

func TestReadTurnModel_NoTurnYet(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"s"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadTurnModel(path); ok || err != nil {
		t.Errorf("ok=%v err=%v, want no turn", ok, err)
	}
}
