package codexsession

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContextUsageSnapshotsAndCompaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	first := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":60,"output_tokens":10},"total_token_usage":{"input_tokens":100,"cached_input_tokens":60,"output_tokens":10,"total_tokens":110},"model_context_window":475000}}}`
	peak := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"cached_input_tokens":150,"output_tokens":20},"total_token_usage":{"input_tokens":300,"cached_input_tokens":210,"output_tokens":30,"total_tokens":330},"model_context_window":475000}}}`
	estimate := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":0,"total_tokens":40},"total_token_usage":{"input_tokens":300,"cached_input_tokens":210,"output_tokens":30,"total_tokens":330}}}}`
	write := func(text string) ContextUsage {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadContextUsage(path)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	base := first + "\n" + peak + "\n" + peak + "\n"
	got := write(base + "torn final line")
	if got.First != 100 || got.Peak != 200 || got.Total.Total != 330 || got.Capacity != 475000 {
		t.Fatalf("snapshots = %+v", got)
	}
	if got.Total.Written != nil || got.Total.Reasoning != nil || *got.Total.Cached != 210 {
		t.Fatalf("missing fields must stay unknown: %+v", got.Total)
	}
	spending := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"cached_input_tokens":150,"output_tokens":20},"total_token_usage":{"total_tokens":350},"model_context_window":475000}}}`
	compacted := base + "{\"type\":\"compacted\"}\n" + peak + "\n" + spending + "\n" + estimate + "\n"
	got = write(compacted)
	if !got.Stale || got.Compactions != 1 || *got.Latest.Input != 200 {
		t.Fatalf("compacted reading = %+v", got)
	}
	got = write(compacted + `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":50},"total_token_usage":{"total_tokens":400}}}}`)
	if got.Stale || got.First != 100 || got.Peak != 200 || *got.Latest.Input != 50 || got.Total.Total != 400 {
		t.Fatalf("fresh reading = %+v", got)
	}
}
