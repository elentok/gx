package cmdtest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// EpicTicketMD is the epic ticket.md SetupEpicSetProject writes.
const EpicTicketMD = "---\nkind: loop\nstatus: draft\nstarted_at: 2026-01-02T03:04:05Z\ncustom: keep-me\n---\n\n# Epic\n\nbody words\n"

// LegacyStatusTicket is a ticket whose status the migrations rewrite.
const LegacyStatusTicket = "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n"

// IsolateTicketStore gives the test a temp HOME and XDG dirs (so neither the
// user's config.json nor their real store is read) and returns the store path.
// The env is available to the test as EnvFor(t).
func IsolateTicketStore(t *testing.T) string {
	t.Helper()
	return filepath.Join(EnvValue(EnvFor(t), "XDG_DATA_HOME"), "gx", "tickets")
}

// AddProject writes <store>/<name>/project.json pointing at repoPath and
// returns the project directory.
func AddProject(t *testing.T, store, name, repoPath string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"name": name, "repo": repoPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// EnsureIssueEpic gives a ticket at <epic>/issues/<file> its epic's ticket.md.
func EnsureIssueEpic(t *testing.T, path string) {
	t.Helper()
	if issuesDir := filepath.Dir(path); filepath.Base(issuesDir) == "issues" {
		testutil.EnsureEpicTicketMD(t, filepath.Dir(issuesDir))
	}
}

func WriteTicketFile(t *testing.T, path, content string) {
	t.Helper()
	EnsureIssueEpic(t, path)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// WriteTicket writes a minimal valid ticket fixture with the given id, status
// and type.
func WriteTicket(t *testing.T, path, id, status, ticketType string) {
	t.Helper()
	EnsureIssueEpic(t, path)
	content := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + ticketType + "\n---\n\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write ticket %s: %v", path, err)
	}
}

func WriteRunLog(t *testing.T, epicDir string, lines []string) {
	t.Helper()
	testutil.Mkdir(t, epicDir)
	path := filepath.Join(epicDir, "run-log.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("writing run-log.jsonl: %v", err)
	}
}

// WriteEpicTicket writes one ticket into scratchDir/<epic>/issues/, creating
// the tracker layout the epic-wide parent-graph checks need in order to find
// an epic at all.
func WriteEpicTicket(t *testing.T, scratchDir, epic, filename, content string) string {
	t.Helper()
	issues := filepath.Join(scratchDir, epic, "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(issues, filename)
	WriteTicketFile(t, path, content)
	return path
}

func TicketFrontmatter(id, extra string) string {
	return "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\n" + extra + "---\nBody.\n"
}

// LogQuestionLine is one open pause.question event line of a ticket log.
const LogQuestionLine = `{"seq":1,"ts":"2026-01-01T00:00:00Z","type":"pause.question","by":"agent","handoff":"h","summary":"Which one?"}` + "\n"

// WriteLogFile writes content as the log beside the ticket at ticketPath.
func WriteLogFile(t *testing.T, ticketPath, content string) {
	t.Helper()
	if err := os.WriteFile(tickets.LogPath(ticketPath), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const showTicketFixture = "---\nid: \"06\"\nstatus: open\ntype: implement\n---\n\n# 06 — Hello\n\nbody text\n"

// SetupShowProject is a project "mine" holding epic/06.
func SetupShowProject(t *testing.T) (repo, project string) {
	t.Helper()
	store := IsolateTicketStore(t)
	repo = testutil.TempRepo(t)
	project = AddProject(t, store, "mine", repo)
	issues := filepath.Join(project, "epic", "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "epic"))
	testutil.WriteFile(t, issues, "06-hello.md", showTicketFixture)
	return repo, project
}

// SetupCloseProject is a project with a follow-ups epic holding open 01, done
// 02, open-with-run-history 03 and cancelled 04, plus an unrelated epic.
func SetupCloseProject(t *testing.T) (repo, project string) {
	t.Helper()
	store := IsolateTicketStore(t)
	repo = testutil.TempRepo(t)
	project = AddProject(t, store, "mine", repo)
	for _, e := range []string{tickets.FollowUpsEpic, "work"} {
		testutil.EnsureEpicTicketMD(t, filepath.Join(project, e))
		if err := os.MkdirAll(filepath.Join(project, e, "issues"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	fu := filepath.Join(project, tickets.FollowUpsEpic, "issues")
	testutil.WriteFile(t, fu, "01-one.md", "---\nid: \"01\"\nstatus: open\ntype: research\n---\n\n# 01 — One\n\nbody\n")
	testutil.WriteFile(t, fu, "02-two.md", "---\nid: \"02\"\nstatus: done\ntype: research\n---\n\n# 02 — Two\n")
	testutil.WriteFile(t, fu, "03-run.md", "---\nid: \"03\"\nstatus: open\ntype: research\nsession_ids:\n  - abc\n---\n\n# 03 — Run\n")
	testutil.WriteFile(t, fu, "04-gone.md", "---\nid: \"04\"\nstatus: cancelled\ntype: research\n---\n\n# 04 — Gone\n")
	testutil.WriteFile(t, filepath.Join(project, "work", "issues"), "01-w.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01\n")
	return repo, project
}

// SetupEpicAddProject is an empty project "mine" with a repo to run in.
func SetupEpicAddProject(t *testing.T) (cwd, project string) {
	t.Helper()
	store := IsolateTicketStore(t)
	repo := testutil.TempRepo(t)
	return repo, AddProject(t, store, "mine", repo)
}

// SetupEpicSetProject is SetupEpicAddProject plus epic "e" holding EpicTicketMD.
// The package test builds its own Deps from the returned Env.
func SetupEpicSetProject(t *testing.T) (env cmdbase.Env, ticketMD string) {
	t.Helper()
	cwd, project := SetupEpicAddProject(t)
	epicDir := filepath.Join(project, "e")
	if err := os.MkdirAll(filepath.Join(epicDir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticketMD = filepath.Join(epicDir, "ticket.md")
	if err := os.WriteFile(ticketMD, []byte(EpicTicketMD), 0o644); err != nil {
		t.Fatal(err)
	}
	return cmdbase.Env{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Getwd:  func() (string, error) { return cwd, nil },
		Env:    EnvFor(t),
	}, ticketMD
}

// AddedPath resolves the address `add` printed back to the stub's file.
func AddedPath(t *testing.T, epicPath, out string) string {
	t.Helper()
	addr := strings.TrimSpace(out)
	id := addr[strings.LastIndex(addr, "/")+1:]
	matches, err := filepath.Glob(filepath.Join(epicPath, "issues", id+"-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("address %q resolves to files %v (err %v), want exactly one", addr, matches, err)
	}
	return matches[0]
}

// MergeTestRepo creates a plain bare repo with a linked worktree for main
// (at mainDir) plus one linked worktree per branch/dir pair in worktrees
// (map of dir name -> branch name); each worktree's branch is created off
// main's tip. It returns the repo dir and main's worktree path.
func MergeTestRepo(t *testing.T, worktrees map[string]string) (repoDir, mainDir string) {
	t.Helper()
	repoDir = testutil.TempBareRepo(t)
	mainDir = filepath.Join(repoDir, "main")
	testutil.MustGitExported(t, repoDir, "worktree", "add", mainDir, "main")

	for dir, branch := range worktrees {
		wtDir := filepath.Join(repoDir, dir)
		testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", branch, wtDir)
		testutil.MustGitExported(t, wtDir, "config", "user.email", "test@test.com")
		testutil.MustGitExported(t, wtDir, "config", "user.name", "Test")
	}
	return repoDir, mainDir
}

func CommitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	testutil.WriteFile(t, dir, name, content)
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add "+name)
}

// RegisterWithTrunk isolates the ticket store and registers repoDir as a
// project whose project.json sets trunk. Not parallel-safe (sets env).
func RegisterWithTrunk(t *testing.T, repoDir, trunk string) (projectDir string) {
	t.Helper()
	projectDir = AddProject(t, IsolateTicketStore(t), "p", repoDir)
	testutil.WriteFile(t, projectDir, "project.json",
		`{"name":"p","repo":"`+repoDir+`","trunk":"`+trunk+`"}`)
	return projectDir
}

// StoreProjectFor isolates the ticket store and registers repoDir as a project
// in it, returning the project directory that holds its epics.
func StoreProjectFor(t *testing.T, repoDir string) string {
	t.Helper()
	return AddProject(t, IsolateTicketStore(t), "mine", repoDir)
}

// WriteStoreProject makes <store>/<name> a project holding one epic with the
// given tickets.
func WriteStoreProject(t *testing.T, store, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	WriteMigrateFixtureEpic(t, dir, "epic", files)
	return dir
}

func WriteMigrateFixtureEpic(t *testing.T, root, epicName string, files map[string]string) string {
	t.Helper()
	issuesDir := filepath.Join(root, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	for name, content := range files {
		WriteTicketFile(t, filepath.Join(issuesDir, name), content)
	}
	return issuesDir
}

func SnapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		out[p] = string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func AssertTreeUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := SnapshotTree(t, root)
	if len(after) != len(before) {
		t.Fatalf("dry run changed the file set: before %d files, after %d", len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("dry run changed %s", p)
		}
	}
}

func WriteSessionFixture(t *testing.T, home, projectSlug, sessionID string, lines []string) string {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", projectSlug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// NonAgentGetwd returns a getwd fake pointed at a plain (non-git) tempdir, so
// git.CurrentBranch fails and checkAgentStatusGuard treats the caller as an
// unrecognised branch — i.e. not an iteration agent.
func NonAgentGetwd(t *testing.T) func() (string, error) {
	t.Helper()
	dir := t.TempDir()
	return func() (string, error) { return dir, nil }
}

// AgentGetwd returns a getwd fake pointed at a real git repo checked out on
// branch, so git.CurrentBranch resolves to it for checkAgentStatusGuard.
func AgentGetwd(t *testing.T, branch string) func() (string, error) {
	t.Helper()
	dir := testutil.TempRepo(t)
	if err := git.CreateBranch(git.Repo{Root: dir}, branch); err != nil {
		t.Fatalf("create branch %s: %v", branch, err)
	}
	return func() (string, error) { return dir, nil }
}

func DecodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output %q is not a JSON object: %v", b, err)
	}
	return m
}

// ShortTempDir keeps the socket path under the unix sun_path limit, which
// t.TempDir() exceeds on macOS.
func ShortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func WaitFor(t *testing.T, ctx context.Context, cond func() bool) {
	t.Helper()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for output")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// SyncBuffer is a bytes.Buffer safe for a writer goroutine and a reading test.
type SyncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *SyncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *SyncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
