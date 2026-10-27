package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestSchemaNamesExactlyTheKinds(t *testing.T) {
	data, err := os.ReadFile("../docs/checks-schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range schema.Properties {
		got = append(got, k)
	}
	slices.Sort(got)
	want := append([]string{"$schema"}, Kinds...)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("schema properties = %v, want %v", got, want)
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Error("schema must set additionalProperties: false")
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"$schema":"x","format":"f {files}","lint":false,"test":{"full":"t"},"typecheck":{"changed":"c","full":"t"}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		"format":    {Changed: "f {files}", Full: "f {files}"},
		"lint":      {Off: true},
		"test":      {Full: "t"},
		"typecheck": {Changed: "c", Full: "t"},
	}
	for k, w := range want {
		if cfg[k] != w {
			t.Errorf("%s = %+v, want %+v", k, cfg[k], w)
		}
	}
	if _, err := ParseConfig([]byte(`{}`)); err != nil {
		t.Errorf("{} must be valid: %v", err)
	}
	for _, bad := range []string{`nope`, `{"build":"x"}`, `{"lint":true}`, `{"lint":1}`, `{"lint":{"changed":"x"}}`, `{"lint":{"full":"x","extra":1}}`, `{"lint":""}`} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func TestExpandBatchesSplitsUnderLimit(t *testing.T) {
	files := []string{"a", "b", "c", "d"}
	got := expandBatches("echo {files}", files, len("echo ")+2*4)
	if len(got) != 2 || got[0] != "echo 'a' 'b'" || got[1] != "echo 'c' 'd'" {
		t.Errorf("batches = %q", got)
	}
	if got := expandBatches("true", files, 1); len(got) != 1 {
		t.Errorf("command without {files}: %q", got)
	}
	if got := expandBatches("echo {files}", []string{"it's"}, 1); got[0] != `echo 'it'\''s'` {
		t.Errorf("quoting = %q", got)
	}
}

func repoWithChecks(t *testing.T, checks string) string {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(repo, ".gx"))
	testutil.WriteFile(t, repo, ".gx/checks.json", checks)
	testutil.CommitAll(t, repo, "checks")
	return repo
}

func TestRunBatchesTogetherListEveryFile(t *testing.T) {
	repo := repoWithChecks(t, `{"format":"echo {files}"}`)
	var want []string
	for _, n := range []string{"one.txt", "two.txt", "three.txt", "four.txt"} {
		testutil.WriteFile(t, repo, n, n)
		want = append(want, n)
	}
	res, err := Run(Options{Dir: repo, DataDir: t.TempDir(), ArgLimit: 40})
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(res.Kinds[0].Log)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(log), "\n"); lines < 2 {
		t.Errorf("want several batches, got %d:\n%s", lines, log)
	}
	for _, n := range want {
		if !strings.Contains(string(log), n) {
			t.Errorf("log lacks %s:\n%s", n, log)
		}
	}
}

func TestRunFailingBatchFailsTheKind(t *testing.T) {
	repo := repoWithChecks(t, `{"format":"for f in {files}; do test \"$f\" != 'three.txt' || exit 1; done"}`)
	for _, n := range []string{"one.txt", "two.txt", "three.txt", "four.txt"} {
		testutil.WriteFile(t, repo, n, n)
	}
	res, err := Run(Options{Dir: repo, DataDir: t.TempDir(), ArgLimit: 80})
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != ExitFail || res.Kinds[0].Status != StatusFail {
		t.Errorf("exit %d, kind %+v", res.Exit, res.Kinds[0])
	}
}
