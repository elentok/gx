package webflows

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestHostFiles_SeedsRepoFilesAndSecondWorktree(t *testing.T) {
	r := start(t, "host-files")
	hf := r.HostFiles
	if hf == nil {
		t.Fatal("no host_files in the output")
	}
	if got := gitOut(t, hf.Repo, "branch", "--show-current"); got != "main" {
		t.Errorf("repo branch = %q, want main checked out", got)
	}
	if list := gitOut(t, hf.Repo, "worktree", "list"); !strings.Contains(list, hf.SecondWorktree) {
		t.Errorf("worktrees:\n%s\nwant %s", list, hf.SecondWorktree)
	}
	if _, err := os.Stat(filepath.Join(hf.SecondWorktree, hf.SecondOnly)); err != nil {
		t.Errorf("second worktree file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hf.Repo, hf.SecondOnly)); err == nil {
		t.Errorf("%s exists in the main worktree too", hf.SecondOnly)
	}
	for _, rel := range []string{"docs/plans/host-files.md", "docs/plans/other.md", ".scratch/flow.mmd", "notes/gone.md", ".env", "doc.pdf", "blob.bin"} {
		if _, err := os.Stat(filepath.Join(hf.Repo, rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	handler := strings.Split(readFile(t, filepath.Join(hf.Repo, "server/files/handler.go")), "\n")
	if len(handler) < 50 || !strings.Contains(handler[41], "handler.go:42") {
		t.Errorf("handler.go has %d lines, line 42 = %q", len(handler), handler[41])
	}
	if st, err := os.Stat(filepath.Join(hf.Repo, "big.log")); err != nil || st.Size() != 3<<20 {
		t.Errorf("big.log = %v, %v, want 3 MB", st, err)
	}
	if svg := readFile(t, filepath.Join(hf.Repo, "logo.svg")); !strings.Contains(svg, "<script>") {
		t.Errorf("logo.svg has no script: %s", svg)
	}
	if target, err := os.Readlink(filepath.Join(hf.Repo, "outside-link")); err != nil || target != "/etc/hosts" {
		t.Errorf("symlink = %q, %v", target, err)
	}
	md := readFile(t, filepath.Join(hf.Repo, "docs/plans/host-files.md"))
	for _, want := range []string{"](../../server/files/handler.go)", "![pic](pic.png)", "```mermaid", "](#install-steps)", "](other.md#install)", "\n## Install steps"} {
		if !strings.Contains(md, want) {
			t.Errorf("host-files.md misses %q", want)
		}
	}
	shot := filepath.Join(hf.TempDir, "shot-1.png")
	st, err := os.Stat(shot)
	if err != nil {
		t.Fatal(err)
	}
	if uid := st.Sys().(*syscall.Stat_t).Uid; int(uid) != os.Getuid() {
		t.Errorf("shot-1.png owner = %d, want %d", uid, os.Getuid())
	}
	if !strings.HasPrefix(readFile(t, shot), "\x89PNG") {
		t.Error("shot-1.png is not a PNG")
	}
}

func TestHostFiles_MockPageAndSecondOrigin(t *testing.T) {
	r := start(t, "host-files")
	hf := r.HostFiles
	mocks := filepath.Join(hf.Repo, "mocks")
	for _, name := range []string{"style.css", "app.js", "data.json", "shot.png"} {
		if _, err := os.Stat(filepath.Join(mocks, name)); err != nil {
			t.Errorf("mocks/%s: %v", name, err)
		}
	}
	page := readFile(t, filepath.Join(mocks, "04-viewer.html"))
	for _, want := range []string{`href="style.css"`, `src="app.js"`, `src="shot.png"`, `src="` + hf.OtherOrigin + `/lib.js"`} {
		if !strings.Contains(page, want) {
			t.Errorf("04-viewer.html misses %q", want)
		}
	}
	app := readFile(t, filepath.Join(mocks, "app.js"))
	for _, want := range []string{"fetch('data.json')", "fetch('/v1/snapshot')", "fetch(other + '/data.json')"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js misses %q", want)
		}
	}
	for path, want := range map[string]string{"/lib.js": "window.otherScript", "/data.json": `"origin":"other"`} {
		resp, err := http.Get(hf.OtherOrigin + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), want) {
			t.Errorf("%s = %d %q, want %q", path, resp.StatusCode, body, want)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s allows cross-origin reads", path)
		}
	}
}

func TestHostFiles_ChatAndTicketHoldTheLinks(t *testing.T) {
	r := start(t, "host-files")
	hf := r.HostFiles
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil || len(list) != 1 {
		t.Fatalf("sessions = %d, %v, want 1", len(list), err)
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", list[0].ID, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	var all []string
	var calls []string
	var prompt bool
	for _, m := range chat.Messages {
		all = append(all, m.Text)
		calls = append(calls, m.Calls...)
		prompt = prompt || (m.Role == "person" && strings.Contains(m.Text, "docs/plans/host-files.md"))
	}
	text := strings.Join(all, "\n")
	for _, link := range []string{"docs/plans/host-files.md", "`~/dev/gx/.scratch/flow.mmd`", "mocks/04-viewer.html", filepath.Join(hf.TempDir, "shot-1.png"), "server/files/handler.go:42", "notes/gone.md"} {
		if !strings.Contains(text, link) {
			t.Errorf("chat misses %q", link)
		}
	}
	if !prompt {
		t.Error("no prompt names a path")
	}
	if strings.Join(calls, "|") != "Read: server/files/handler.go" {
		t.Errorf("calls = %v", calls)
	}
	var round *wire.QuestionCard
	for i := range chat.Questions {
		if chat.Questions[i].Source == wire.CardSourceRound {
			round = &chat.Questions[i]
		}
	}
	if round == nil || len(round.Questions) != 1 || len(round.Questions[0].Options) != 2 || !strings.Contains(round.Questions[0].Body, "docs/plans/host-files.md") {
		t.Errorf("round card = %+v, want one question with a path and two options", round)
	}
	if body := r.Gx(t, "tickets", "show", hf.Ticket); !strings.Contains(body, "`docs/plans/host-files.md`") || !strings.Contains(body, filepath.Join(hf.Repo, ".scratch", "flow.mmd")) {
		t.Errorf("ticket body lacks a relative and an absolute path:\n%s", body)
	}
}

func TestHostFiles_NoMainWorktreeShapeChecksOutTrunkNowhere(t *testing.T) {
	r := startWith(t, "host-files", nil, "--shape", "no-main-worktree")
	hf := r.HostFiles
	if got := gitOut(t, hf.Repo, "branch", "--show-current"); got == "main" || got == "" {
		t.Errorf("repo branch = %q, want a branch other than main", got)
	}
	if list := gitOut(t, hf.Repo, "worktree", "list"); strings.Contains(list, "[main]") {
		t.Errorf("a worktree has main checked out:\n%s", list)
	}
	if _, err := os.Stat(filepath.Join(hf.SecondWorktree, hf.SecondOnly)); err != nil {
		t.Errorf("second worktree file: %v", err)
	}
}
