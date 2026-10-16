package agents

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

const createMaxWidth = 80

var createHint = ui.RenderInlineBindings(
	key.NewBinding(key.WithHelp("tab/shift+tab", "field")),
	key.NewBinding(key.WithHelp("↑/↓", "choose")),
	key.NewBinding(key.WithHelp("enter", "create")),
	key.NewBinding(key.WithHelp("esc", "close")),
)

// newWorktreeLabel is the dir entry that asks for a worktree name instead.
const newWorktreeLabel = "new worktree…"

// formProject is a project the create form offers: its root and the dirs of
// its other worktrees.
type formProject struct {
	Name      string
	Root      string
	Worktrees []string
}

// CreatePrefill pre-fills the create form; every field stays editable.
type CreatePrefill struct {
	Project, Dir, Name, Prompt, Ticket string
}

type createField int

const (
	fieldProject createField = iota
	fieldDir
	fieldWorktree
	fieldName
	fieldPrompt
)

// createForm is the create-session form. projects is nil until loaded; dir
// indexes dirs(), where the last entry is "new worktree…".
type createForm struct {
	prefill  CreatePrefill
	projects []formProject
	loadErr  string
	project  int
	dir      int
	field    createField
	worktree string
	name     string
	prompt   string
	err      string
	sending  bool
}

type formProjectsMsg struct {
	projects []formProject
	err      error
}

type sessionCreatedMsg struct {
	result server.SessionResult
	err    error
}

// listFormProjects reads the ticket store's projects and each one's worktrees.
func listFormProjects() ([]formProject, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	dirs, err := tickets.ProjectDirs(cfg.TicketStore.Path)
	if err != nil {
		return nil, err
	}
	var out []formProject
	for _, d := range dirs {
		pf, err := config.ReadProjectFile(d)
		if err != nil || pf.Repo == nil {
			continue
		}
		repo, err := git.FindRepo(*pf.Repo)
		if err != nil {
			continue
		}
		p := formProject{Name: tickets.ProjectName(d), Root: repo.Root}
		wts, _ := git.ListWorktrees(*repo)
		for _, wt := range wts {
			if filepath.Clean(wt.Path) != filepath.Clean(repo.Root) {
				p.Worktrees = append(p.Worktrees, wt.Path)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// openCreate opens the form on the project in focus: the selected session's.
func (m Model) openCreate() (Model, tea.Cmd) {
	p := CreatePrefill{}
	if m.cursor < len(m.sessions) {
		p.Project = m.sessions[m.cursor].Project
	}
	return m.OpenCreateForm(p)
}

// OpenCreateForm opens the create form pre-filled with p; without a client
// there is no server to create on.
func (m Model) OpenCreateForm(p CreatePrefill) (Model, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	m.create = &createForm{prefill: p, name: p.Name, prompt: p.Prompt, field: fieldDir}
	list := m.listProjects
	return m, func() tea.Msg {
		projects, err := list()
		return formProjectsMsg{projects: projects, err: err}
	}
}

func (m Model) updateFormProjects(msg formProjectsMsg) (tea.Model, tea.Cmd) {
	if m.create == nil {
		return m, nil
	}
	f := *m.create
	m.create = &f
	if msg.err != nil {
		f.loadErr = msg.err.Error()
		return m, nil
	}
	f.projects = msg.projects
	if i := slices.IndexFunc(f.projects, func(p formProject) bool { return p.Name == f.prefill.Project }); i >= 0 {
		f.project = i
	} else if f.prefill.Project != "" {
		f.projects = append([]formProject{{Name: f.prefill.Project}}, f.projects...)
	}
	if f.prefill.Dir != "" {
		f.dir = f.dirIndex(f.prefill.Dir)
	}
	return m, nil
}

// dirIndex is dir's entry, adding it first to the project's dirs when absent.
func (f *createForm) dirIndex(dir string) int {
	dirs := f.dirs()
	if i := slices.IndexFunc(dirs, func(d string) bool { return filepath.Clean(d) == filepath.Clean(dir) }); i >= 0 {
		return i
	}
	p := &f.projects[f.project]
	p.Worktrees = append([]string{dir}, p.Worktrees...)
	if p.Root == "" {
		return 0
	}
	return 1
}

// dirs is the selected project's root, then its worktrees; "new worktree…"
// follows them.
func (f *createForm) dirs() []string {
	if len(f.projects) == 0 {
		return nil
	}
	p := f.projects[f.project]
	var out []string
	if p.Root != "" {
		out = append(out, p.Root)
	}
	return append(out, p.Worktrees...)
}

func (f *createForm) newWorktree() bool { return f.dir == len(f.dirs()) }

// fields is the form's fields in tab order; the worktree name only shows
// for a new worktree.
func (f *createForm) fields() []createField {
	if f.newWorktree() {
		return []createField{fieldProject, fieldDir, fieldWorktree, fieldName, fieldPrompt}
	}
	return []createField{fieldProject, fieldDir, fieldName, fieldPrompt}
}

func (f *createForm) moveField(step int) {
	fields := f.fields()
	i := max(0, slices.Index(fields, f.field))
	f.field = fields[(i+step+len(fields))%len(fields)]
}

func (f *createForm) text() *string {
	switch f.field {
	case fieldWorktree:
		return &f.worktree
	case fieldName:
		return &f.name
	case fieldPrompt:
		return &f.prompt
	}
	return nil
}

func (m Model) updateCreateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := *m.create
	m.create = &f
	switch msg.String() {
	case "esc":
		m.create = nil
	case "tab":
		f.moveField(1)
	case "shift+tab":
		f.moveField(-1)
	case "down", "ctrl+n":
		f.choose(1)
	case "up", "ctrl+p":
		f.choose(-1)
	case "enter":
		return m.submitCreate()
	case "backspace":
		if t := f.text(); t != nil {
			if r := []rune(*t); len(r) > 0 {
				*t = string(r[:len(r)-1])
			}
		}
	default:
		if t := f.text(); t != nil && msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			*t += msg.Text
		}
	}
	return m, nil
}

// choose steps the project or dir choice, wrapping; a new project starts on
// its root.
func (f *createForm) choose(step int) {
	switch f.field {
	case fieldProject:
		if n := len(f.projects); n > 0 {
			f.project = (f.project + step + n) % n
			f.dir = 0
		}
	case fieldDir:
		if len(f.projects) > 0 {
			n := len(f.dirs()) + 1
			f.dir = (f.dir + step + n) % n
		}
	}
}

func (m Model) submitCreate() (tea.Model, tea.Cmd) {
	f := m.create
	if f.sending || len(f.projects) == 0 {
		return m, nil
	}
	req := server.CreateSessionRequest{
		Project: f.projects[f.project].Name,
		Name:    strings.TrimSpace(f.name),
		Prompt:  f.prompt,
		Ticket:  f.prefill.Ticket,
	}
	if f.newWorktree() {
		req.Worktree = strings.TrimSpace(f.worktree)
		if req.Worktree == "" {
			f.err = "a new worktree needs a name"
			return m, nil
		}
	} else {
		req.Dir = f.dirs()[f.dir]
	}
	// The new session has no info lines yet, so it gets the whole panel.
	req.Cols, req.Rows = max(m.mainWidth()-2, 1), max(m.height-2, 1)
	f.sending, f.err = true, ""
	client := m.client
	return m, func() tea.Msg {
		res, err := client.CreateSession(context.Background(), req)
		return sessionCreatedMsg{result: res, err: err}
	}
}

// updateSessionCreated closes the form and selects the new session once a
// snapshot lists it; a failure stays in the form's footer.
func (m Model) updateSessionCreated(msg sessionCreatedMsg) (tea.Model, tea.Cmd) {
	if m.create == nil {
		return m, nil
	}
	f := *m.create
	m.create = &f
	f.sending = false
	switch {
	case msg.err != nil:
		f.err = msg.err.Error()
		return m, nil
	case msg.result.Refused || msg.result.Session == nil:
		f.err = msg.result.Message
		if f.err == "" {
			f.err = "create refused: " + msg.result.Reason
		}
		return m, nil
	}
	m.create = nil
	m.pendingSelect = msg.result.Session.ID
	return m.selectPending()
}

// selectPending refreshes, first selecting and focusing the just-created
// session when it is listed.
func (m Model) selectPending() (tea.Model, tea.Cmd) {
	i := slices.IndexFunc(m.sessions, func(s server.SessionInfo) bool { return s.ID == m.pendingSelect })
	if m.pendingSelect == "" || i < 0 {
		return m.refresh()
	}
	m.pendingSelect = ""
	return m.selectAndFocus(i)
}

// dirSessions is how many live sessions run in dir.
func (m Model) dirSessions(dir string) int {
	n := 0
	for _, s := range m.sessions {
		if s.Status.State != server.SessionExited && filepath.Clean(s.Dir) == filepath.Clean(dir) {
			n++
		}
	}
	return n
}

func (m Model) renderCreate() string {
	f := m.create
	width := min(createMaxWidth, m.width-4)
	innerW := width - 4
	label := func(fl createField, name string) string {
		if f.field == fl {
			return ui.StyleTitle.Render("› " + name)
		}
		return ui.StyleMuted.Render("  " + name)
	}
	textLine := func(fl createField, name, value string) string {
		cursor := ""
		if f.field == fl {
			cursor = "▏"
		}
		return fit(fit(label(fl, name), 12)+value+cursor, innerW)
	}
	var lines []string
	switch {
	case f.loadErr != "":
		lines = append(lines, styleCrashed.Render(f.loadErr))
	case f.projects == nil:
		lines = append(lines, ui.StyleMuted.Render("loading projects…"))
	case len(f.projects) == 0:
		lines = append(lines, ui.StyleMuted.Render("no projects in the ticket store"))
	default:
		lines = append(lines, fit(label(fieldProject, "Project"), 12)+styleProjectHeader.Render(f.projects[f.project].Name), "")
		lines = append(lines, label(fieldDir, "Dir"))
		entries := append(f.dirs(), "")
		for i, d := range entries {
			text := "    " + newWorktreeLabel
			if i < len(entries)-1 {
				text = "    " + shortDir(d)
				if n := m.dirSessions(d); n > 0 {
					text += ui.StyleMuted.Render("  " + plural(n, "session"))
				}
			}
			row := fit(text, innerW)
			if i == f.dir {
				row = ui.RenderRowHighlight(row)
			}
			lines = append(lines, row)
		}
		lines = append(lines, "")
		if f.newWorktree() {
			lines = append(lines, textLine(fieldWorktree, "Worktree", f.worktree))
		}
		lines = append(lines, textLine(fieldName, "Name", f.name), textLine(fieldPrompt, "Prompt", f.prompt))
		if f.prefill.Ticket != "" {
			lines = append(lines, fit(ui.StyleMuted.Render("  Ticket")+"    "+styleTicket.Render(f.prefill.Ticket), innerW))
		}
	}
	hint := createHint
	switch {
	case f.err != "":
		hint = styleCrashed.Render(f.err)
	case f.sending:
		hint = ui.StyleMuted.Render("creating…")
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "New session",
		Body:          strings.Join(lines, "\n"),
		Hint:          hint,
		Width:         width,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
