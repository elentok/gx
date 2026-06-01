package git

import "testing"

func TestRepoRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cwd     string
		rawPath string
		repo    string
		want    string
		wantErr bool
	}{
		{
			name:    "relative path from repo root",
			cwd:     "/repo",
			rawPath: "git/log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "relative path from a subdirectory",
			cwd:     "/repo/git",
			rawPath: "log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "already repo-relative-looking path resolved from subdir",
			cwd:     "/repo/cmd",
			rawPath: "../git/log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "absolute path under the root",
			cwd:     "/repo/cmd",
			rawPath: "/repo/git/log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "dot-slash prefix is stripped",
			cwd:     "/repo",
			rawPath: "./git/log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "traversal that stays inside the repo",
			cwd:     "/repo/cmd/sub",
			rawPath: "../../git/log.go",
			repo:    "/repo",
			want:    "git/log.go",
		},
		{
			name:    "repo root itself resolves to dot",
			cwd:     "/repo",
			rawPath: ".",
			repo:    "/repo",
			want:    ".",
		},
		{
			name:    "path escaping the repo root is rejected",
			cwd:     "/repo/git",
			rawPath: "../../outside/file.go",
			repo:    "/repo",
			wantErr: true,
		},
		{
			name:    "absolute path outside the root is rejected",
			cwd:     "/repo",
			rawPath: "/elsewhere/file.go",
			repo:    "/repo",
			wantErr: true,
		},
		{
			name:    "non-existent path is still valid input",
			cwd:     "/repo",
			rawPath: "does/not/exist.go",
			repo:    "/repo",
			want:    "does/not/exist.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RepoRelativePath(tt.cwd, tt.rawPath, tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RepoRelativePath(%q, %q, %q) = %q, want error", tt.cwd, tt.rawPath, tt.repo, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RepoRelativePath(%q, %q, %q) returned error: %v", tt.cwd, tt.rawPath, tt.repo, err)
			}
			if got != tt.want {
				t.Fatalf("RepoRelativePath(%q, %q, %q) = %q, want %q", tt.cwd, tt.rawPath, tt.repo, got, tt.want)
			}
		})
	}
}
