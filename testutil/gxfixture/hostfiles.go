package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/hostfileseed"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/transcript"
)

const (
	hostFilesSecondOnly = "only-in-feature.md"
	// hostFilesOutside is the symlink target: a file outside every known root.
	hostFilesOutside = "/etc/hosts"
)

// setupHostFiles lays out the host-files scenario in demo's repo (which seed
// made and committed): the repo files, the viewer mock, a second worktree with
// a file of its own, the temp folder, the ticket body and the second origin. The
// no-main-worktree shape ends by moving the repo off main, so no worktree has it.
//
// It lives on its own type so fixture stays under the sizeguard method limit.
type hostFilesFixture struct{ *fixture }

func (f hostFilesFixture) setup(shape string) error {
	repo := filepath.Join(f.root, "repo")
	pic, err := hostfileseed.PNG()
	if err != nil {
		return err
	}
	other, err := hostfileseed.StartOtherOrigin()
	if err != nil {
		return err
	}
	files, err := hostfileseed.Files(pic)
	if err != nil {
		return err
	}
	page, app := hostfileseed.ViewerMock(other)
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
