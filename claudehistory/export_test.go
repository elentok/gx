package claudehistory

import (
	"strings"
	"testing"
)

// fixtureExport covers: role headings, thinking (collapsible), tool_use (compact JSON),
// tool_result (truncated), sidechain marking, and noise stripping.
const fixtureExport = `{"type":"mode","mode":"normal","sessionId":"export-test"}
{"type":"ai-title","aiTitle":"Export Test","sessionId":"export-test"}
{"parentUuid":null,"isSidechain":false,"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>ignore this</local-command-caveat>"},"timestamp":"2026-01-01T10:00:00Z"}
{"parentUuid":null,"isSidechain":false,"type":"user","isMeta":false,"message":{"role":"user","content":"<command-name>/beads</command-name>\n<command-message>beads</command-message>\n<command-args>x1</command-args>"},"timestamp":"2026-01-01T10:00:01Z"}
{"parentUuid":null,"isSidechain":false,"type":"user","isMeta":false,"message":{"role":"user","content":"Hello, please help me."},"timestamp":"2026-01-01T10:00:02Z"}
{"parentUuid":null,"isSidechain":false,"type":"assistant","isMeta":false,"message":{"role":"assistant","content":[{"type":"thinking","thinking":"I should help the user."},{"type":"text","text":"Sure, let me run a command."},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls -la"}}]},"timestamp":"2026-01-01T10:00:03Z"}
{"parentUuid":null,"isSidechain":false,"type":"user","isMeta":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file1.txt\nfile2.txt","is_error":false}]},"timestamp":"2026-01-01T10:00:04Z"}
{"parentUuid":null,"isSidechain":false,"type":"assistant","isMeta":false,"message":{"role":"assistant","content":[{"type":"text","text":"Done."}]},"timestamp":"2026-01-01T10:00:05Z"}
{"parentUuid":null,"isSidechain":true,"type":"assistant","isMeta":false,"message":{"role":"assistant","content":[{"type":"text","text":"Subagent response."}]},"timestamp":"2026-01-01T10:00:06Z"}
{"parentUuid":null,"isSidechain":false,"type":"system","subtype":"turn_duration","durationMs":100,"timestamp":"2026-01-01T10:00:07Z"}
`

func TestExportMarkdownRoleHeadings(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## User\n") {
		t.Errorf("expected ## User heading\ngot:\n%s", out)
	}
	if !strings.Contains(out, "## Assistant\n") {
		t.Errorf("expected ## Assistant heading\ngot:\n%s", out)
	}
}

func TestExportMarkdownNoiseStripped(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	// isMeta record and command-wrapper should not appear
	if strings.Contains(out, "local-command-caveat") {
		t.Errorf("local-command-caveat noise leaked into output")
	}
	if strings.Contains(out, "command-name") {
		t.Errorf("command-name noise leaked into output")
	}
	// system record should not appear
	if strings.Contains(out, "turn_duration") {
		t.Errorf("system record leaked into output")
	}
}

func TestExportMarkdownThinkingCollapsible(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<details>") {
		t.Errorf("thinking block missing <details> tag\ngot:\n%s", out)
	}
	if !strings.Contains(out, "<summary>Thinking</summary>") {
		t.Errorf("thinking block missing <summary>Thinking</summary>\ngot:\n%s", out)
	}
	if !strings.Contains(out, "I should help the user.") {
		t.Errorf("thinking content missing\ngot:\n%s", out)
	}
	if !strings.Contains(out, "</details>") {
		t.Errorf("thinking block missing </details>\ngot:\n%s", out)
	}
}

func TestExportMarkdownToolUseCompact(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "**Bash**") {
		t.Errorf("tool_use name missing\ngot:\n%s", out)
	}
	if !strings.Contains(out, "```json") {
		t.Errorf("tool_use fenced json block missing\ngot:\n%s", out)
	}
	if !strings.Contains(out, `"command"`) {
		t.Errorf("tool_use input key missing\ngot:\n%s", out)
	}
}

func TestExportMarkdownToolResult(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "**Result:**") {
		t.Errorf("tool_result prefix missing\ngot:\n%s", out)
	}
	if !strings.Contains(out, "file1.txt") {
		t.Errorf("tool_result content missing\ngot:\n%s", out)
	}
}

func TestExportMarkdownToolResultTruncated(t *testing.T) {
	// Build a tool_result with more than 50 lines
	var lines []string
	for range 80 {
		lines = append(lines, "line")
	}
	bigResult := strings.Join(lines, `\n`) // JSON-escaped newline
	fixture := `{"type":"user","isMeta":false,"isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":"` +
		bigResult + `","is_error":false}]},"timestamp":"2026-01-01T10:00:00Z"}` + "\n"

	path := writeFixture(t, fixture)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "... (truncated)") {
		t.Errorf("expected truncation marker\ngot:\n%s", out)
	}
}

func TestExportMarkdownSidechainMarked(t *testing.T) {
	path := writeFixture(t, fixtureExport)
	out, err := ExportMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## Assistant (subagent)") {
		t.Errorf("sidechain turn not marked\ngot:\n%s", out)
	}
	if !strings.Contains(out, "Subagent response.") {
		t.Errorf("sidechain content missing\ngot:\n%s", out)
	}
}
