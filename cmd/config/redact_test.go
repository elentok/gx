package config

import "testing"

type redactHook struct {
	Name  string `json:"name"`
	Token string `json:"token" secret:"true"`
}

type redactCfg struct {
	Hooks  []redactHook          `json:"hooks"`
	ByName map[string]redactHook `json:"by-name"`
	Plain  string                `json:"plain"`
}

func TestToTree_RedactsSecretsInsideSlicesAndMaps(t *testing.T) {
	cfg := redactCfg{
		Hooks:  []redactHook{{Name: "a", Token: "s1"}},
		ByName: map[string]redactHook{"x": {Name: "x", Token: "s2"}},
		Plain:  "p",
	}
	tree, paths, err := toTree(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	m := tree.(map[string]any)
	hook := m["hooks"].([]any)[0].(map[string]any)
	if hook["token"] != redacted || hook["name"] != "a" {
		t.Errorf("slice element not masked correctly: %v", hook)
	}
	byName := m["by-name"].(map[string]any)["x"].(map[string]any)
	if byName["token"] != redacted {
		t.Errorf("map value not masked: %v", byName)
	}
	want := map[string]bool{"hooks.0.token": true, "by-name.x.token": true}
	if len(paths) != 2 || !want[paths[0]] || !want[paths[1]] {
		t.Errorf("paths = %v", paths)
	}
}

func TestTouchesSecret(t *testing.T) {
	paths := []string{"hooks.0.token"}
	for key, want := range map[string]bool{
		"hooks.0.token": true,
		"hooks.0":       true,
		"hooks":         true,
		"hooks.0.name":  false,
		"plain":         false,
	} {
		if got := touchesSecret(key, paths); got != want {
			t.Errorf("touchesSecret(%q) = %v, want %v", key, got, want)
		}
	}
}
