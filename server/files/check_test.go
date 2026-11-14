package files

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

type env struct {
	base, repo, outside, tmp, state, data, store string
	roots                                        []string
	extra                                        []string
	c                                            *Checker
}

func write(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// newEnv builds fixtures with symlink-resolved paths (macOS temp is a symlink).
func newEnv(t *testing.T) *env {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		base:    base,
		repo:    filepath.Join(base, "home", "repo"),
		outside: filepath.Join(base, "home", "other"),
		tmp:     filepath.Join(base, "tmp"),
		state:   filepath.Join(base, "state"),
		data:    filepath.Join(base, "data"),
	}
	e.store = filepath.Join(e.data, "tickets")
	for _, d := range []string{e.repo, e.outside, e.tmp, e.state, e.store} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.roots = []string{e.repo, e.store}
	e.c = &Checker{
		BuiltinRoots: func() []string { return e.roots },
		ExtraRoots:   func() []string { return e.extra },
		TempRoots:    func() []string { return []string{e.tmp} },
		DeniedDirs:   func() []string { return []string{e.state, filepath.Join(e.base, "config")} },
		DataDir:      func() string { return e.data },
		TicketsStore: func() string { return e.store },
		UID:          uint32(os.Getuid()),
	}
	return e
}

func TestCheckHosted(t *testing.T) {
	e := newEnv(t)
	extra := filepath.Join(e.base, "extra")
	e.extra = []string{extra}
	for name, p := range map[string]string{
		"repo":        write(t, filepath.Join(e.repo, "docs", "plan.md")),
		"extra":       write(t, filepath.Join(extra, "a.md")),
		"pub key":     write(t, filepath.Join(e.repo, "keys", "id_rsa.pub")),
		"ticket mock": write(t, filepath.Join(e.store, "epic", "mocks", "m.html")),
		"temp":        write(t, filepath.Join(e.tmp, "sub", "ok.md")),
	} {
		h, ok := e.c.Check(p)
		if !ok || h.Path != p || h.Size != 1 || h.ModTime.IsZero() {
			t.Errorf("%s: Check(%q) = %+v, %v; want hosted", name, p, h, ok)
		}
	}
}

func TestCheckNotHosted(t *testing.T) {
	e := newEnv(t)
	inRepo := write(t, filepath.Join(e.repo, "a.md"))
	secret := write(t, filepath.Join(e.repo, ".env"))
	out := write(t, filepath.Join(e.outside, "o.md"))
	if err := os.Symlink(out, filepath.Join(e.repo, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(e.repo, "notes.md")); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"relative":        "docs/a.md",
		"missing":         filepath.Join(e.repo, "nope.md"),
		"directory":       e.repo,
		"dotdot":          filepath.Join(e.repo, "..", "other", "o.md"),
		"outside root":    out,
		"outside symlink": filepath.Join(e.repo, "link.md"),
		"deny symlink":    filepath.Join(e.repo, "notes.md"),
		"state":           write(t, filepath.Join(e.state, "web-auth.json")),
		"web-tls":         write(t, filepath.Join(e.state, "web-tls", "cert")),
		"config":          write(t, filepath.Join(e.base, "config", "c.toml")),
		"sessions":        write(t, filepath.Join(e.data, "sessions", "s.jsonl")),
	}
	// a data folder file stays denied even when it is also a root
	e.roots = append(e.roots, e.data)
	maps.Copy(paths, map[string]string{
		".env":             secret,
		".ENV":             write(t, filepath.Join(e.repo, ".ENV")),
		".env.local":       write(t, filepath.Join(e.repo, ".env.local")),
		".git":             write(t, filepath.Join(e.repo, ".git", "config")),
		".ssh":             write(t, filepath.Join(e.repo, ".ssh", "known_hosts")),
		".aws":             write(t, filepath.Join(e.repo, ".aws", "credentials")),
		".npmrc":           write(t, filepath.Join(e.repo, ".npmrc")),
		".netrc":           write(t, filepath.Join(e.repo, ".netrc")),
		".git-credentials": write(t, filepath.Join(e.repo, ".git-credentials")),
		".gnupg":           write(t, filepath.Join(e.repo, ".gnupg", "k")),
		".kube":            write(t, filepath.Join(e.repo, ".kube", "config")),
		".docker":          write(t, filepath.Join(e.repo, ".docker", "config.json")),
		".pgpass":          write(t, filepath.Join(e.repo, ".pgpass")),
		"pem":              write(t, filepath.Join(e.repo, "a.PEM")),
		"key":              write(t, filepath.Join(e.repo, "a.key")),
		"p12":              write(t, filepath.Join(e.repo, "a.p12")),
		"pfx":              write(t, filepath.Join(e.repo, "a.pfx")),
		"id_rsa":           write(t, filepath.Join(e.repo, "id_rsa")),
		"id_ed25519_work":  write(t, filepath.Join(e.repo, "id_ed25519_work")),
		"id_ecdsa":         write(t, filepath.Join(e.repo, "ID_ECDSA")),
		"id_dsa":           write(t, filepath.Join(e.repo, "id_dsa")),
	})
	for name, p := range paths {
		if h, ok := e.c.Check(p); ok {
			t.Errorf("%s: Check(%q) = %+v; want not hosted", name, p, h)
		}
	}
	if _, ok := e.c.Check(inRepo); !ok {
		t.Errorf("control file %q should be hosted", inRepo)
	}
}

func TestCheckBuiltinRootsReadEachCall(t *testing.T) {
	e := newEnv(t)
	wt := write(t, filepath.Join(e.base, "worktree", "a.md"))
	if _, ok := e.c.Check(wt); ok {
		t.Fatal("worktree hosted before it was added")
	}
	e.roots = append(e.roots, filepath.Dir(wt))
	if _, ok := e.c.Check(wt); !ok {
		t.Fatal("worktree added later should be hosted")
	}
}

func TestCheckTempRules(t *testing.T) {
	e := newEnv(t)
	foreignFile := write(t, filepath.Join(e.tmp, "foreign.md"))
	inForeignDir := write(t, filepath.Join(e.tmp, "theirs", "deep", "a.md"))
	linked := write(t, filepath.Join(e.tmp, "one.md"))
	if err := os.Link(linked, filepath.Join(e.tmp, "two.md")); err != nil {
		t.Fatal(err)
	}
	e.c.statOverride = func(p string, info os.FileInfo) (uint32, uint64, bool) {
		uid, nlink, ok := sysStat(info)
		switch p {
		case foreignFile, filepath.Join(e.tmp, "theirs"):
			uid++
		}
		return uid, nlink, ok
	}
	for name, p := range map[string]string{
		"foreign file":   foreignFile,
		"foreign folder": inForeignDir,
		"link count 2":   linked,
	} {
		if _, ok := e.c.Check(p); ok {
			t.Errorf("%s: %q should not be hosted", name, p)
		}
	}
	// the same hard link in a repo is fine
	repoFile := write(t, filepath.Join(e.repo, "a.md"))
	if err := os.Link(repoFile, filepath.Join(e.repo, "b.md")); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.c.Check(filepath.Join(e.repo, "b.md")); !ok {
		t.Error("hard link inside a repo should be hosted")
	}
}

func TestCheckGxFoldersInsideTempRoot(t *testing.T) {
	e := newEnv(t)
	e.state = filepath.Join(e.tmp, "state")
	e.data = filepath.Join(e.tmp, "data")
	e.store = filepath.Join(e.data, "tickets")
	e.roots = []string{e.repo}
	for _, p := range []string{
		write(t, filepath.Join(e.state, "web-auth.json")),
		write(t, filepath.Join(e.data, "sessions", "s.jsonl")),
		write(t, filepath.Join(e.state, "web-tls")),
	} {
		if _, ok := e.c.Check(p); ok {
			t.Errorf("%q should not be hosted", p)
		}
	}
	mock := write(t, filepath.Join(e.store, "epic", "mocks", "m.html"))
	if _, ok := e.c.Check(mock); !ok {
		t.Errorf("ticket mock %q should be hosted", mock)
	}
}
