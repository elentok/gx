package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/transcript"
)

const (
	hostFilesSecondOnly = "only-in-feature.md"
	// hostFilesOutside is the symlink target: a file outside every known root.
	hostFilesOutside = "/etc/hosts"
	hostFilesBigLog  = 3 << 20
)

// hostFilesPNG is a small real PNG (a gradient), so a viewer has dimensions to show.
func hostFilesPNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 40))
	for y := range 40 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 6), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

// startOtherOrigin serves one script and one JSON path on another loopback
// port, with no CORS headers, for the rest of the fixture's life. It returns its URL.
func startOtherOrigin() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/lib.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write([]byte("window.otherScript = true;\n"))
	})
	mux.HandleFunc("/data.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"origin":"other"}`))
	})
	go func() { _ = http.Serve(ln, mux) }() //nolint:gosec // loopback fixture listener
	return "http://" + ln.Addr().String(), nil
}

// hostFilesFiles is every file of demo's repo, by path; the viewer mock is added
// separately because it needs the second origin's URL.
func hostFilesFiles(pic []byte) (map[string][]byte, error) {
	var handler strings.Builder
	handler.WriteString("package files\n\n")
	for i := 3; i <= 60; i++ {
		switch i {
		case 42:
			handler.WriteString("// ServeFile is the line the chat links to (handler.go:42).\n")
		default:
			fmt.Fprintf(&handler, "// line %d\n", i)
		}
	}
	var intro strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&intro, "Filler paragraph %d, so the heading below sits under the fold.\n\n", i)
	}
	plan := "# Host files plan\n\n" +
		"See [the handler](../../server/files/handler.go) and the picture ![pic](pic.png).\n\n" +
		"Jump to [the install steps](#install-steps) or read [other.md#install](other.md#install).\n\n" +
		"```mermaid\nflowchart LR\n  Agent --> Link --> Viewer\n```\n\n" +
		intro.String() + "## Install steps\n\nRun the installer.\n"
	return map[string][]byte{
		"docs/plans/host-files.md": []byte(plan),
		"docs/plans/other.md":      []byte("# Other\n\n## Install\n\nThe install section.\n"),
		"docs/plans/pic.png":       pic,
		".scratch/flow.mmd":        []byte("flowchart LR\n  A[Agent] --> B[gx files url] --> C[Chat link] --> D[Viewer]\n"),
		"server/files/handler.go":  []byte(handler.String()),
		"notes/gone.md":            []byte("# Gone\n\nThe flow deletes this after the chat loads.\n"),
		".env":                     []byte("SECRET=hunter2\n"),
		"big.log":                  bytes.Repeat([]byte("a log line of some length\n"), hostFilesBigLog/26+1)[:hostFilesBigLog],
		"doc.pdf":                  []byte("%PDF-1.1\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 100]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"),
		"blob.bin":                 {0, 1, 2, 3, 0, 255, 254, 0, 7},
		"logo.svg":                 []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><script>window.svgRan = true;</script><circle cx="20" cy="20" r="18"/></svg>` + "\n"),
		"mocks/style.css":          []byte("body { font-family: sans-serif; }\n#title { color: rgb(1, 2, 3); }\n"),
		"mocks/data.json":          []byte(`{"same":"origin"}` + "\n"),
		"mocks/shot.png":           pic,
	}, nil
}

// viewerMock is mocks/04-viewer.html and its app.js. The page records five
// behaviours in #results as JSON: the page loads its CSS, script and image; a
// same-origin fetch of data.json works; a read of /v1/snapshot is refused; a
// fetch to the second origin fails; a script from the second origin loads.
func viewerMock(other string) (page, app []byte) {
	page = []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>Viewer mock</title>
<link rel="stylesheet" href="style.css"></head>
<body data-other="` + other + `">
<h1 id="title">Viewer mock</h1>
<img id="shot" src="shot.png" alt="shot">
<pre id="results"></pre>
<script src="app.js"></script>
<script src="` + other + `/lib.js"></script>
</body></html>
`)
	app = []byte(`const results = {};
const other = document.body.dataset.other;
const attempt = (name, p) => p.then((v) => { results[name] = v; }, () => { results[name] = 'failed'; });
Promise.all([
  attempt('sameOriginFetch', fetch('data.json').then((r) => r.json()).then(() => 'ok')),
  attempt('snapshot', fetch('/v1/snapshot').then((r) => (r.ok ? 'read' : 'refused'))),
  attempt('otherOriginFetch', fetch(other + '/data.json').then(() => 'ok')),
]).then(() => {
  window.addEventListener('load', () => {
    results.otherScript = window.otherScript === true ? 'loaded' : 'missing';
    results.image = document.getElementById('shot').naturalWidth > 0 ? 'loaded' : 'missing';
    results.style = getComputedStyle(document.getElementById('title')).color;
    document.getElementById('results').textContent = JSON.stringify(results);
    document.body.dataset.done = '1';
  }, { once: true });
  if (document.readyState === 'complete') window.dispatchEvent(new Event('load'));
});
`)
	return page, app
}

// setupHostFiles lays out the host-files scenario in demo's repo (which seed
// made and committed): the repo files, the viewer mock, a second worktree with
// a file of its own, the temp folder, the ticket body and the second origin. The
// no-main-worktree shape ends by moving the repo off main, so no worktree has it.
//
// It lives on its own type so fixture stays under the sizeguard method limit.
type hostFilesFixture struct{ *fixture }

func (f hostFilesFixture) setup(shape string) error {
	repo := filepath.Join(f.root, "repo")
	pic, err := hostFilesPNG()
	if err != nil {
		return err
	}
	other, err := startOtherOrigin()
	if err != nil {
		return err
	}
	files, err := hostFilesFiles(pic)
	if err != nil {
		return err
	}
	page, app := viewerMock(other)
	files["mocks/04-viewer.html"], files["mocks/app.js"] = page, app
	// The chat's ~/ link: the fixture's home holds the same diagram.
	files["../home/dev/gx/.scratch/flow.mmd"] = files[".scratch/flow.mmd"]
	for rel, data := range files {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	if err := os.Symlink(hostFilesOutside, filepath.Join(repo, "outside-link")); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "host files"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	repo, second, err := f.layoutWorktrees(repo, shape)
	if err != nil {
		return err
	}
	temp := filepath.Join(f.root, "shots")
	if err := os.MkdirAll(temp, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, "shot-1.png"), pic, 0o644); err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%s/%s", project, spec.HostFilesEpic, spec.HostFilesTicket)
	body := "The plan is `docs/plans/host-files.md` and the diagram is `" + filepath.Join(repo, ".scratch", "flow.mmd") + "`.\n"
	if out, err := f.cli("tickets", "section", addr, "Links", body); err != nil {
		return fmt.Errorf("ticket body: %w\n%s", err, out)
	}
	f.hostFiles = &output.HostFilesOutput{Repo: repo, SecondWorktree: second, SecondOnly: hostFilesSecondOnly, TempDir: temp, OtherOrigin: other, Ticket: addr}
	return nil
}

// layoutWorktrees adds the feature worktree, which alone holds hostFilesSecondOnly,
// and returns the checkout the repo's files are read from and the feature worktree.
// In the no-main-worktree shape the project's repo becomes a bare repo with its
// worktrees as siblings, none on main, so ticketgit.TrunkCheckout finds no trunk
// checkout; the returned checkout is then the "elsewhere" worktree, not the repo.
func (f hostFilesFixture) layoutWorktrees(repo, shape string) (checkout, second string, err error) {
	checkout, second = repo, filepath.Join(repo, ".worktrees", "feature")
	if shape == "no-main-worktree" {
		gitDir, aside := filepath.Join(repo, ".git"), repo+".aside"
		for _, step := range []func() error{
			func() error { return os.Rename(gitDir, aside) },
			func() error { return os.RemoveAll(repo) },
			func() error { return os.Rename(aside, repo) },
			func() error { return f.git(repo, "config", "--bool", "core.bare", "true") },
		} {
			if err := step(); err != nil {
				return "", "", err
			}
		}
		checkout, second = repo+"-elsewhere", repo+"-feature"
		if err := f.git(repo, "worktree", "add", "-b", "elsewhere", checkout, "main"); err != nil {
			return "", "", err
		}
	}
	if err := f.git(repo, "worktree", "add", "-b", "feature", second, "main"); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(second, hostFilesSecondOnly), []byte("# Only here\n"), 0o644); err != nil {
		return "", "", err
	}
	for _, args := range [][]string{{"add", hostFilesSecondOnly}, {"commit", "-m", "only in the feature worktree"}} {
		if err := f.git(second, args...); err != nil {
			return "", "", err
		}
	}
	return checkout, second, nil
}

// hostFilesChat is the chat the scenario appends to the session's transcript:
// a prompt naming a path, a tool call, the mock's message with every link, and
// a question card (the last agent text, which is what makes a printed round).
func (f sessionSeeder) hostFilesChat() []string {
	hf := f.hostFiles
	line := func(kind, id string, blocks ...map[string]any) string {
		raw, _ := json.Marshal(map[string]any{
			"type": kind, "uuid": "hf-" + id, "timestamp": "2026-01-01T10:00:00Z",
			"message": map[string]any{"content": blocks},
		})
		return string(raw)
	}
	text := func(s string) map[string]any { return map[string]any{"type": "text", "text": s} }
	links := "I wrote the plan to docs/plans/host-files.md and a diagram at `~/dev/gx/.scratch/flow.mmd`. " +
		"The mock is mocks/04-viewer.html, a screenshot is " + filepath.Join(hf.TempDir, "shot-1.png") +
		" and the handler is in server/files/handler.go:42. The old file notes/gone.md was deleted."
	round := "❓ **Q1** - **Which file next?**: Open docs/plans/host-files.md or the handler?\n\n➡️ The plan\n\n" +
		"```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: \"The plan\"\n  - id: b\n    label: \"The handler\"\n```"
	return []string{
		line("user", "prompt", text("Please read docs/plans/host-files.md and tell me about the handler")),
		line("assistant", "tool", map[string]any{"type": "tool_use", "id": "toolu_hf1", "name": "Read", "input": map[string]any{"file_path": "server/files/handler.go"}}),
		line("assistant", "links", text(links)),
		line("assistant", "round", text(round)),
	}
}

// hostFilesSecondText is the second session's message; it names a file only
// the feature worktree has.
const hostFilesSecondText = "I added " + hostFilesSecondOnly + " in this worktree."

// seedHostFilesSecondChat appends one message to the feature-worktree session
// and waits until the chat shows it.
func (f sessionSeeder) seedHostFilesSecondChat(id, harness string) error {
	if harness == "codex" {
		return fmt.Errorf("host-files chat is Claude only")
	}
	raw, _ := json.Marshal(map[string]any{
		"type": "assistant", "uuid": "hf-second", "timestamp": "2026-01-01T10:00:00Z",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": hostFilesSecondText}}},
	})
	if err := f.appendChatLines(id, harness, []string{string(raw)}); err != nil {
		return err
	}
	return f.waitFor(id, "the second worktree's message", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return slices.ContainsFunc(c.Messages, func(m wire.SessionChatMessage) bool { return strings.Contains(m.Text, hostFilesSecondOnly) })
	})
}

// seedHostFilesChat appends the host-files messages and waits until the chat
// shows the tool call and the question card.
func (f sessionSeeder) seedHostFilesChat(id, harness string) error {
	if harness == "codex" {
		return fmt.Errorf("host-files chat is Claude only")
	}
	if err := f.appendChatLines(id, harness, f.hostFilesChat()); err != nil {
		return err
	}
	return f.waitFor(id, "the host-files messages", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		tool := slices.ContainsFunc(c.Messages, func(m wire.SessionChatMessage) bool {
			return m.Kind == transcript.ChatKindTools && slices.Contains(m.Calls, "Read: server/files/handler.go")
		})
		card := slices.ContainsFunc(c.Questions, func(q wire.QuestionCard) bool { return q.Source == wire.CardSourceRound })
		return tool && card
	})
}
