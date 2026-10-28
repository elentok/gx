package syshealth

import "testing"

func TestDisplayName(t *testing.T) {
	cases := []struct {
		name string
		comm string
		argv []string
		want string
	}{
		{"go", "go", []string{"go", "test", "./..."}, "go test"},
		{"npm", "npm", []string{"npm", "run", "build"}, "npm run"},
		{"pnpm flag first", "pnpm", []string{"pnpm", "-C", "webui", "test"}, "pnpm webui"},
		{"yarn alone", "yarn", []string{"yarn"}, "yarn"},
		{"bun", "bun", []string{"bun", "install"}, "bun install"},
		{"git", "git", []string{"/usr/bin/git", "-C", "x", "status"}, "git x"},
		{"cargo", "cargo", []string{"cargo", "build"}, "cargo build"},
		{"make", "make", []string{"make", "-j4", "all"}, "make all"},
		{"node script", "node", []string{"node", "/a/b/cli.js"}, "cli.js"},
		{"sh script", "sh", []string{"sh", "./x.sh"}, "x.sh"},
		{"bash bare", "bash", []string{"bash"}, "bash"},
		{"zsh bare", "zsh", []string{"-zsh"}, "-zsh"},
		{"python", "python3.12", []string{"python3.12", "/x/y.py"}, "y.py"},
		{"sh -c", "sh", []string{"sh", "-c", "echo hi"}, "sh"},
		{"node -e", "node", []string{"node", "-e", "1+1"}, "node"},
		{"interpreter flags only", "node", []string{"node", "--inspect"}, "node"},
		{"other", "build.py", []string{"/usr/local/bin/ripgrep", "foo"}, "ripgrep"},
		{"claude", "2.1.0", []string{"claude"}, "claude"},
		{"no argv", "launchd", nil, "launchd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DisplayName(Process{Comm: c.comm, Argv: c.argv})
			if got != c.want {
				t.Errorf("DisplayName(%q) = %q, want %q", c.argv, got, c.want)
			}
		})
	}
}

func TestIsClaude(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want bool
	}{
		{"argv0 base", []string{"/usr/local/bin/claude", "--resume"}, true},
		{"native installer path", []string{"/Users/d/.local/share/claude/versions/2.1.0", "-p"}, true},
		{"node cli.js", []string{"node", "/x/node_modules/@anthropic-ai/claude-code/cli.js"}, true},
		{"other cli.js", []string{"node", "/x/node_modules/other/cli.js"}, false},
		{"relative versions path", []string{"claude/versions/2.1.0"}, false},
		{"unrelated", []string{"go", "test"}, false},
		{"no argv", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsClaude(Process{Argv: c.argv}); got != c.want {
				t.Errorf("IsClaude(%q) = %v, want %v", c.argv, got, c.want)
			}
		})
	}
}
