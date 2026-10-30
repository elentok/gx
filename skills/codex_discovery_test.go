package skills

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This optional contract check uses the installed Codex binary without a model
// turn, credentials, or the user's configuration. CI without Codex skips it.
func TestCodexDiscoversDirectorySkillAndSiblingReferences(t *testing.T) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI not installed")
	}
	base := t.TempDir()
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(home, ".codex")
	work := filepath.Join(base, "work")
	for _, path := range []string{codexHome, work} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(base, "source")
	skill := writeSourceFile(t, src, "gx-directory-probe/SKILL.md", "---\nname: gx-directory-probe\ndescription: Isolated gx directory discovery probe.\n---\nRead ../gx-shared-probe.md and support.txt.\n")
	support := writeSourceFile(t, src, "gx-directory-probe/support.txt", "support probe")
	shared := writeSourceFile(t, src, "gx-shared-probe.md", "shared probe")
	mustInstall(t, filepath.Join(base, "manifest.json"), InstallRequest{AgentRoots: []string{CodexSkillsRoot(home)}, Mode: ModeSymlink, Files: []SourceFile{skill, support, shared}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "debug", "prompt-input", "probe")
	cmd.Dir = work
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + codexHome, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"), "XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "TMPDIR=" + base, "PATH=" + filepath.Dir(binary)}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("isolated Codex prompt-input: %v", err)
	}
	// Codex emits a skill-root alias and a relative skill path. Expand exactly the
	// advertised path, then resolve the same sibling references a skill will use.
	var prompt any
	if err := json.Unmarshal(output, &prompt); err != nil {
		t.Fatal(err)
	}
	var instructions string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case string:
			if strings.Contains(value, "gx-directory-probe/SKILL.md") {
				instructions = value
			}
		case []any:
			for _, v := range value {
				visit(v)
			}
		case map[string]any:
			for _, v := range value {
				visit(v)
			}
		}
	}
	visit(prompt)
	match := regexp.MustCompile(`\(file: (r[0-9]+)/gx-directory-probe/SKILL.md\)`).FindStringSubmatch(instructions)
	if len(match) != 2 {
		t.Fatalf("Codex did not advertise directory-linked skill: %s", instructions)
	}
	rootMatch := regexp.MustCompile("(?m)^- `" + match[1] + "` = `([^`]+)`").FindStringSubmatch(instructions)
	if len(rootMatch) != 2 {
		t.Fatal("missing advertised discovery root")
	}
	advertised := filepath.Join(rootMatch[1], "gx-directory-probe", "SKILL.md")
	if readFile(t, advertised) != readFile(t, skill.AbsPath) {
		t.Fatal("advertised skill path does not resolve")
	}
	if readFile(t, filepath.Join(filepath.Dir(advertised), "support.txt")) != "support probe" {
		t.Fatal("support file does not resolve")
	}
	if readFile(t, filepath.Join(filepath.Dir(advertised), "..", "gx-shared-probe.md")) != "shared probe" {
		t.Fatal("shared reference does not resolve")
	}
}
