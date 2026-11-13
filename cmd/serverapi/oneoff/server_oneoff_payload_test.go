package oneoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

func writePayload(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApplyOneOffPayload_FrontmatterFillsFieldsAndFlagsWin(t *testing.T) {
	t.Parallel()
	path := writePayload(t, "---\nexpected_context_window: 40000\nname: from-file\nagent: codex\nproject: other\nblocked_by: [p:e/01]\nfront: true\n---\nbuild it from the file\n")
	req := wire.OneOffRequest{ExpectedContextWindow: 90000, Agent: "claude"}
	if refusal := applyOneOffPayload(&req, path); refusal != nil {
		t.Fatalf("refused: %+v", refusal)
	}
	if req.ExpectedContextWindow != 90000 || req.Agent != "claude" {
		t.Errorf("flags should win: %+v", req)
	}
	if req.Name != "from-file" || req.Project != "other" || !req.Front || len(req.BlockedBy) != 1 {
		t.Errorf("frontmatter should fill the rest: %+v", req)
	}
	if !strings.Contains(req.Prompt, "build it from the file") {
		t.Errorf("prompt = %q; want the body", req.Prompt)
	}
}

func TestApplyOneOffPayload_PromptArgumentBeatsTheBody(t *testing.T) {
	t.Parallel()
	req := wire.OneOffRequest{Prompt: "from the argument"}
	if refusal := applyOneOffPayload(&req, writePayload(t, "the body\n")); refusal != nil {
		t.Fatalf("refused: %+v", refusal)
	}
	if req.Prompt != "from the argument" {
		t.Errorf("prompt = %q", req.Prompt)
	}
}

func TestApplyOneOffPayload_TypeKeyIsIgnored(t *testing.T) {
	t.Parallel()
	var req wire.OneOffRequest
	if refusal := applyOneOffPayload(&req, writePayload(t, "---\ntype: implement\n---\njust a prompt\n")); refusal != nil {
		t.Fatalf("refused: %+v", refusal)
	}
	if req.Type != "" {
		t.Errorf("type key set the request type to %q", req.Type)
	}
}

func TestApplyOneOffPayload_MissingFileIsAnInvalidFileRefusal(t *testing.T) {
	t.Parallel()
	var req wire.OneOffRequest
	refusal := applyOneOffPayload(&req, filepath.Join(t.TempDir(), "missing.md"))
	if refusal == nil || !refusal.Refused || refusal.Reason != wire.ReasonBadFile {
		t.Errorf("got %+v; want a %s refusal", refusal, wire.ReasonBadFile)
	}
}

func TestApplyOneOffPayload_RemovedKeysAreRefused(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"commits", "base"} {
		var req wire.OneOffRequest
		refusal := applyOneOffPayload(&req, writePayload(t, "---\n"+key+": main\n---\nbuild it\n"))
		if refusal == nil || refusal.Reason != wire.ReasonOptionRemoved {
			t.Fatalf("%s: got %+v; want a %s refusal", key, refusal, wire.ReasonOptionRemoved)
		}
		if !strings.Contains(refusal.Message, key) || !strings.Contains(refusal.Message, "never commits") {
			t.Errorf("message %q should name %q and say one-offs never commit", refusal.Message, key)
		}
	}
}

func TestApplyOneOffPayload_UniqueKeyIsTheResolvedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "payload.md")
	if err := os.WriteFile(file, []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"file": file, "symlink": link} {
		var req wire.OneOffRequest
		if refusal := applyOneOffPayload(&req, path); refusal != nil {
			t.Fatalf("%s: refused: %+v", name, refusal)
		}
		if req.Unique != resolved {
			t.Errorf("%s: unique = %q; want %q", name, req.Unique, resolved)
		}
	}
}

func TestApplyOneOffPayload_UniqueKeyOrder(t *testing.T) {
	t.Parallel()
	path := writePayload(t, "---\nunique: from-file\n---\nbody\n")
	var req wire.OneOffRequest
	if applyOneOffPayload(&req, path) != nil || req.Unique != "from-file" {
		t.Errorf("frontmatter key: unique = %q; want from-file", req.Unique)
	}
	req = wire.OneOffRequest{Unique: "from-flag"}
	if applyOneOffPayload(&req, path) != nil || req.Unique != "from-flag" {
		t.Errorf("flag: unique = %q; want from-flag", req.Unique)
	}
	req = wire.OneOffRequest{NoUnique: true}
	if applyOneOffPayload(&req, writePayload(t, "body\n")) != nil || req.Unique != "" {
		t.Errorf("--no-unique: unique = %q; want none", req.Unique)
	}
}

// A payload refusal goes through the same render path as the server's, so
// --json and the exit code do not change with where the file is read.
func TestRenderOneOff_PayloadRefusalNeedsNoClient(t *testing.T) {
	t.Parallel()
	var req wire.OneOffRequest
	refusal := applyOneOffPayload(&req, writePayload(t, "---\ncommits: main\n---\nbuild it\n"))
	if refusal == nil {
		t.Fatal("want a refusal")
	}
	var out, errOut strings.Builder
	err := renderOneOff(t.Context(), config.Dirs{}, nil, &out, &errOut, true, true, 0, *refusal)
	if err == nil || !strings.Contains(err.Error(), wire.ReasonOptionRemoved) {
		t.Errorf("err = %v; want a refusal naming %s", err, wire.ReasonOptionRemoved)
	}
	if !strings.Contains(out.String(), `"reason": "`+wire.ReasonOptionRemoved+`"`) {
		t.Errorf("--json stdout = %q; want the refusal object", out.String())
	}
}
