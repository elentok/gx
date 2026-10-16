package agents

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

const createMaxWidth = 80

// createChrome is the form's height outside the picker rows: the frame,
// hint, Where line, blank lines and the text fields.
const createChrome = 12

const maxPickerRows = 16

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

// dirs is the project's root, then its worktrees.
func (p formProject) dirs() []string {
	var out []string
	if p.Root != "" {
		out = append(out, p.Root)
	}
	return append(out, p.Worktrees...)
}

// formEntry is one choice in the Where picker: a dir of a project, or a new
// worktree in it when dir is empty.
type formEntry struct {
	project int
	dir     string
}

func (e formEntry) newWorktree() bool { return e.dir == "" }

// CreatePrefill pre-fills the create form; every field stays editable.
type CreatePrefill struct {
	Project, Dir, Name, Prompt, Ticket string
}

type createField int

const (
	fieldWhere createField = iota
	fieldWorktree
	fieldName
	fieldPrompt
)

// createForm is the create-session form. projects is nil until loaded, and
// lists the focused project first. matches is the entries query keeps; sel
// indexes it and offset is the first picker row shown.
type createForm struct {
	prefill  CreatePrefill
	projects []formProject
	loadErr  string
	field    createField
	query    string
	matches  []formEntry
	sel      int
	offset   int
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

// WithCwdProject names the project of the repo gx was opened in.
func (m Model) WithCwdProject(name string) Model {
	m.cwdProject = name
	return m
}

// openCreate opens the form on the project in focus: the selected session's,
// else the cwd repo's; with neither the form falls back to the first project.
func (m Model) openCreate() (Model, tea.Cmd) {
	p := CreatePrefill{Project: m.cwdProject}
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
	m.create = &createForm{prefill: p, name: p.Name, prompt: p.Prompt, field: fieldWhere}
	list := m.listProjects
	return m, func() tea.Msg {
		projects, err := list()
		return formProjectsMsg{projects: projects, err: err}
	}
}

// updateFormProjects moves the pre-filled project first, adding it when the
// store lacks it, and selects the pre-filled dir, adding it when absent.
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
	f.projects = slices.Clone(msg.projects)
	if f.projects == nil {
		f.projects = []formProject{}
	}
	if i := slices.IndexFunc(f.projects, func(p formProject) bool { return p.Name == f.prefill.Project }); i >= 0 {
		p := f.projects[i]
		f.projects = append([]formProject{p}, slices.Delete(f.projects, i, i+1)...)
	} else if f.prefill.Project != "" {
		f.projects = append([]formProject{{Name: f.prefill.Project}}, f.projects...)
	}
	dir := f.prefill.Dir
	if dir != "" && len(f.projects) > 0 {
		p := &f.projects[0]
		if !slices.ContainsFunc(p.dirs(), func(d string) bool { return filepath.Clean(d) == filepath.Clean(dir) }) {
			p.Worktrees = append([]string{dir}, p.Worktrees...)
		}
	}
	f.filter()
	if dir != "" {
		f.sel = max(0, slices.IndexFunc(f.matches, func(e formEntry) bool {
			return e.project == 0 && filepath.Clean(e.dir) == filepath.Clean(dir)
		}))
	}
	f.scroll(m.pickerRows())
	return m, nil
}

// filter keeps the entries matching query, in project order, and selects
// the first.
func (f *createForm) filter() {
	f.matches = nil
	for i, p := range f.projects {
		for _, d := range append(p.dirs(), "") {
			e := formEntry{project: i, dir: d}
			if fuzzyMatch(f.query, f.entryText(e)) {
				f.matches = append(f.matches, e)
			}
		}
	}
	f.sel, f.offset = 0, 0
}

// entryText is what the query matches an entry against.
func (f *createForm) entryText(e formEntry) string {
	label := newWorktreeLabel
	if !e.newWorktree() {
		label = shortDir(e.dir)
	}
	return f.projects[e.project].Name + " " + label
}

// fuzzyMatch reports whether each space-separated term of query appears in
// text in order, ignoring case.
func fuzzyMatch(query, text string) bool {
	text = strings.ToLower(text)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		rest := text
		for _, r := range term {
			i := strings.IndexRune(rest, r)
			if i < 0 {
				return false
			}
			rest = rest[i+len(string(r)):]
		}
	}
	return true
}

// selected is the chosen entry; false when nothing matches.
func (f *createForm) selected() (formEntry, bool) {
	if f.sel < len(f.matches) {
		return f.matches[f.sel], true
	}
	return formEntry{}, false
}

// selectedProject is the chosen entry's project name.
func (f *createForm) selectedProject() string {
	if e, ok := f.selected(); ok {
		return f.projects[e.project].Name
	}
	return ""
}

func (f *createForm) newWorktree() bool {
	e, ok := f.selected()
	return ok && e.newWorktree()
}

// pickerRow is a project header (entry < 0) or a match.
type pickerRow struct {
	project int
	entry   int
}

// rows is the picker's rows: each project with matches gets a header.
func (f *createForm) rows() []pickerRow {
	var out []pickerRow
	for i, e := range f.matches {
		if i == 0 || f.matches[i-1].project != e.project {
			out = append(out, pickerRow{project: e.project, entry: -1})
		}
		out = append(out, pickerRow{project: e.project, entry: i})
	}
	return out
}

// scroll keeps the selection, and its header when it is a group's first
// entry, within the n visible rows.
func (f *createForm) scroll(n int) {
	rows := f.rows()
	at := slices.IndexFunc(rows, func(r pickerRow) bool { return r.entry == f.sel })
	if at < 0 {
		f.offset = 0
		return
	}
	sel := at
	if at > 0 && rows[at-1].entry < 0 {
		at--
	}
	f.offset = max(min(f.offset, at), sel-n+1)
	f.offset = max(0, min(f.offset, len(rows)-n))
}

// pickerRows is how many picker rows fit the screen.
func (m Model) pickerRows() int {
	return max(3, min(maxPickerRows, m.height-createChrome))
}

// fields is the form's fields in tab order; the worktree name only shows
// for a new worktree.
func (f *createForm) fields() []createField {
	if f.newWorktree() {
		return []createField{fieldWhere, fieldWorktree, fieldName, fieldPrompt}
	}
	return []createField{fieldWhere, fieldName, fieldPrompt}
}

func (f *createForm) moveField(step int) {
	fields := f.fields()
	i := max(0, slices.Index(fields, f.field))
	f.field = fields[(i+step+len(fields))%len(fields)]
}

func (f *createForm) text() *string {
	switch f.field {
	case fieldWhere:
		return &f.query
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
	query := f.query
	switch msg.String() {
	case "esc":
		m.create = nil
		return m, nil
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
	if f.query != query && f.projects != nil {
		f.filter()
	}
	f.scroll(m.pickerRows())
	return m, nil
}

// choose steps the Where choice, wrapping.
func (f *createForm) choose(step int) {
	if n := len(f.matches); f.field == fieldWhere && n > 0 {
		f.sel = (f.sel + step + n) % n
	}
}

func (m Model) submitCreate() (tea.Model, tea.Cmd) {
	f := m.create
	if f.sending || len(f.projects) == 0 {
		return m, nil
	}
	e, ok := f.selected()
	if !ok {
		f.err = "nothing matches " + strconv.Quote(f.query)
		return m, nil
	}
	req := server.CreateSessionRequest{
		Project: f.projects[e.project].Name,
		Name:    strings.TrimSpace(f.name),
		Prompt:  f.prompt,
		Ticket:  f.prefill.Ticket,
	}
	if e.newWorktree() {
		req.Worktree = strings.TrimSpace(f.worktree)
		if req.Worktree == "" {
			f.err = "a new worktree needs a name"
			return m, nil
		}
	} else {
		req.Dir = e.dir
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
	textLine := func(fl createField, name, value string, w int) string {
		cursor := ""
		if f.field == fl {
			cursor = "▏"
		}
		return fit(fit(label(fl, name), 12)+value+cursor, w)
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
		count := ui.StyleMuted.Render(strconv.Itoa(len(f.matches)))
		lines = append(lines, textLine(fieldWhere, "Where", f.query, innerW-6)+fitLeft(count, 6), "")
		lines = append(lines, m.renderPicker(innerW)...)
		lines = append(lines, "")
		if f.newWorktree() {
			lines = append(lines, textLine(fieldWorktree, "Worktree", f.worktree, innerW))
		}
		lines = append(lines, textLine(fieldName, "Name", f.name, innerW), textLine(fieldPrompt, "Prompt", f.prompt, innerW))
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

// renderPicker is the visible picker rows: project headers, then their dirs
// and a new-worktree entry.
func (m Model) renderPicker(innerW int) []string {
	f := m.create
	rows := f.rows()
	if len(rows) == 0 {
		return []string{ui.StyleMuted.Render("    no matches")}
	}
	end := min(len(rows), f.offset+m.pickerRows())
	var lines []string
	for _, r := range rows[f.offset:end] {
		if r.entry < 0 {
			lines = append(lines, fit("  "+styleProjectHeader.Render(f.projects[r.project].Name), innerW))
			continue
		}
		e := f.matches[r.entry]
		text := "    " + newWorktreeLabel
		if !e.newWorktree() {
			text = "    " + shortDir(e.dir)
			if n := m.dirSessions(e.dir); n > 0 {
				text += ui.StyleMuted.Render("  " + plural(n, "session"))
			}
		}
		row := fit(text, innerW)
		if r.entry == f.sel {
			row = ui.RenderRowHighlight(row)
		}
		lines = append(lines, row)
	}
	return lines
}

// fitLeft right-aligns s in w cells.
func fitLeft(s string, w int) string {
	if pad := w - ansi.StringWidth(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
