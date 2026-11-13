package config

import (
	"slices"
	"testing"
)

func TestWritableDirectoriesProjectAddsToGlobal(t *testing.T) {
	global := Config{Agents: DefaultAgentsConfig()}
	project := global.WithProject(ProjectFile{Agents: &AgentsConfig{WritableDirectories: []string{"/project/cache"}}})
	if !slices.Equal(project.Agents.WritableDirectories, []string{"~/Library/pnpm", "/project/cache"}) {
		t.Fatalf("directories = %v", project.Agents.WritableDirectories)
	}
	if !slices.Equal(global.Agents.WritableDirectories, []string{"~/Library/pnpm"}) {
		t.Fatal("project changed global directories")
	}
}

func TestParseWritableDirectories(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{`{}`, []string{"~/Library/pnpm"}},
		{`{"agents":{"writable-directories":[]}}`, []string{}},
		{`{"agents":{"writable-directories":["/cache"]}}`, []string{"/cache"}},
	} {
		cfg, err := Parse([]byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(cfg.Agents.WritableDirectories, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.input, cfg.Agents.WritableDirectories, tc.want)
		}
	}
	pf, err := ParseProjectFile([]byte(`{"agents":{"writable-directories":["/project/cache"]}}`))
	if err != nil || pf.Agents == nil || !slices.Equal(pf.Agents.WritableDirectories, []string{"/project/cache"}) {
		t.Fatalf("project directories = %+v, %v", pf.Agents, err)
	}
}
