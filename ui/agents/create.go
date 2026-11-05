package agents

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/wire"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
	"github.com/elentok/gx/ui/fuzzy"
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
	score   int
	pos     []int // matched rune indices of the entry's label
}

func (e formEntry) newWorktree() bool { return e.dir == "" }

// CreatePrefill pre-fills the create form; every field stays editable.
type CreatePrefill struct {
	Project, Dir, Name, Prompt, Ticket string
	// RebaseNote and MadeWorktree ride along to the created session.
	RebaseNote   string
	MadeWorktree bool
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
	home     string // collapses to ~ in entry labels
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
	result wire.SessionResult
	err    error
}

// listFormProjects reads the ticket store's projects and each one's worktrees.
func listFormProjects(ticketStore string) ([]formProject, error) {
	dirs, err := tickets.ProjectDirs(ticketStore)
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
	field := fieldWhere
	if p.Dir != "" {
		field = fieldName
	}
	m.create = &createForm{home: m.settings.Home(), prefill: p, name: p.Name, prompt: p.Prompt, field: field}
	return m, m.create.load(m.listProjects)
}

// createCtx is what the form reads from the Model: screen sizes and the
// session API that creates sessions.
type createCtx struct {
	height, mainWidth int
	sessions          Sessions
}

func (c createCtx) pickerRows() int {
	return max(3, min(maxPickerRows, c.height-createChrome))
}

// createResult tells the Model what a form update decided.
type createResult struct {
	closed    bool   // the form is done and the Model drops it
	createdID string // the new session to select once a snapshot lists it
}

// load is the command that lists the projects the form offers.
func (*createForm) load(list func() ([]formProject, error)) tea.Cmd {
	return func() tea.Msg {
		projects, err := list()
		return formProjectsMsg{projects: projects, err: err}
	}
}

// updateProjects moves the pre-filled project first, adding it when the
// store lacks it, and selects the pre-filled dir, adding it when absent.
func (c *createForm) updateProjects(msg formProjectsMsg, ctx createCtx) *createForm {
	f := *c
	if msg.err != nil {
		f.loadErr = msg.err.Error()
		return &f
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
	f.scroll(ctx.pickerRows())
	return &f
}

// filter keeps the entries matching query and selects the first. Projects
// rank by their best match and dirs by score within a project, ties keeping
// project order; each project's new-worktree entry stays last.
func (f *createForm) filter() {
	type group struct {
		best    int
		entries []formEntry
	}
	var groups []group
	for i, p := range f.projects {
		var g group
		for _, d := range append(p.dirs(), "") {
			e := formEntry{project: i, dir: d}
			if f.match(&e) {
				if len(g.entries) == 0 || e.score > g.best {
					g.best = e.score
				}
				g.entries = append(g.entries, e)
			}
		}
		if len(g.entries) > 0 {
			slices.SortStableFunc(g.entries, func(a, b formEntry) int {
				if a.newWorktree() != b.newWorktree() {
					return boolCmp(a.newWorktree(), b.newWorktree())
				}
				return b.score - a.score
			})
			groups = append(groups, g)
		}
	}
	slices.SortStableFunc(groups, func(a, b group) int { return b.best - a.best })
	f.matches = nil
	for _, g := range groups {
		f.matches = append(f.matches, g.entries...)
	}
	f.sel, f.offset = 0, 0
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// match scores e against query over "project label", keeping the matched
// positions that fall in the label.
func (f *createForm) match(e *formEntry) bool {
	name := f.projects[e.project].Name
	score, pos, ok := fuzzy.Score(f.query, name+" "+entryLabel(f.home, *e))
	if !ok {
		return false
	}
	e.score = score
	skip := len([]rune(name)) + 1
	for _, p := range pos {
		if p >= skip {
			e.pos = append(e.pos, p-skip)
		}
	}
	return true
}

// entryLabel is how the picker shows e.
func entryLabel(home string, e formEntry) string {
	if e.newWorktree() {
		return newWorktreeLabel
	}
	return agentstyle.ShortDir(home, e.dir)
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

// Update handles a key press.
func (c *createForm) Update(msg tea.KeyPressMsg, ctx createCtx) (*createForm, tea.Cmd, createResult) {
	cp := *c
	f := &cp
	query := f.query
	switch msg.String() {
	case "esc":
		return nil, nil, createResult{closed: true}
	case "tab":
		f.moveField(1)
	case "shift+tab":
		f.moveField(-1)
	case "down", "ctrl+n":
		f.choose(1)
	case "up", "ctrl+p":
		f.choose(-1)
	case "enter":
		return f, f.submit(ctx), createResult{}
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
	f.scroll(ctx.pickerRows())
	return f, nil, createResult{}
}

// choose steps the Where choice, wrapping.
func (f *createForm) choose(step int) {
	if n := len(f.matches); f.field == fieldWhere && n > 0 {
		f.sel = (f.sel + step + n) % n
	}
}

func (f *createForm) submit(ctx createCtx) tea.Cmd {
	if f.sending || len(f.projects) == 0 {
		return nil
	}
	e, ok := f.selected()
	if !ok {
		f.err = "nothing matches " + strconv.Quote(f.query)
		return nil
	}
	req := wire.CreateSessionRequest{
		Project: f.projects[e.project].Name,
		Name:    strings.TrimSpace(f.name),
		Prompt:  f.prompt,
		Ticket:  f.prefill.Ticket,
	}
	if e.newWorktree() {
		req.Worktree = strings.TrimSpace(f.worktree)
		if req.Worktree == "" {
			f.err = "a new worktree needs a name"
			return nil
		}
	} else {
		req.Dir = e.dir
		// The note and trust flag belong to the prefilled worktree only.
		if f.prefill.Dir != "" && e.dir == f.prefill.Dir {
			req.MadeWorktree, req.RebaseNote = f.prefill.MadeWorktree, f.prefill.RebaseNote
		}
	}
	// The new session has no info lines yet, so it gets the whole panel.
	req.Cols, req.Rows = max(ctx.mainWidth-2, 1), max(ctx.height-1, 1)
	f.sending, f.err = true, ""
	sessions := ctx.sessions
	return func() tea.Msg {
		res, err := sessions.Create(context.Background(), req)
		return sessionCreatedMsg{result: res, err: err}
	}
}

// updateCreated closes the form once the session exists; a failure stays in
// the form's footer.
func (c *createForm) updateCreated(msg sessionCreatedMsg) (*createForm, createResult) {
	f := *c
	f.sending = false
	switch {
	case msg.err != nil:
		f.err = msg.err.Error()
		return &f, createResult{}
	case msg.result.Refused || msg.result.Session == nil:
		f.err = msg.result.Message
		if f.err == "" {
			f.err = "create refused: " + msg.result.Reason
		}
		return &f, createResult{}
	}
	return nil, createResult{closed: true, createdID: msg.result.Session.ID}
}

// selectPending refreshes, first selecting and focusing the just-created
// session when it is listed.
func (m Model) selectPending() (tea.Model, tea.Cmd) {
	i := slices.IndexFunc(m.sessions, func(s wire.SessionInfo) bool { return s.ID == m.pendingSelect })
	if m.pendingSelect == "" || i < 0 {
		return m.refresh()
	}
	m.pendingSelect = ""
	return m.selectAndFocus(i)
}

// dirSessions is how many live sessions run in dir.
func dirSessions(sessions []wire.SessionInfo, dir string) int {
	n := 0
	for _, s := range sessions {
		if s.Status.State != wire.SessionExited && filepath.Clean(s.Dir) == filepath.Clean(dir) {
			n++
		}
	}
	return n
}

// view renders the form over a screen of the given size; sessions feed the
// per-dir session counts.
func (f *createForm) view(screenW, screenH int, sessions []wire.SessionInfo) string {
	width := min(createMaxWidth, screenW-4)
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
		return agentstyle.Fit(agentstyle.Fit(label(fl, name), 12)+value+cursor, w)
	}
	var lines []string
	switch {
	case f.loadErr != "":
		lines = append(lines, agentstyle.Crashed.Render(f.loadErr))
	case f.projects == nil:
		lines = append(lines, ui.StyleMuted.Render("loading projects…"))
	case len(f.projects) == 0:
		lines = append(lines, ui.StyleMuted.Render("no projects in the ticket store"))
	default:
		count := ui.StyleMuted.Render(strconv.Itoa(len(f.matches)))
		lines = append(lines, textLine(fieldWhere, "Where", f.query, innerW-6)+fitLeft(count, 6), "")
		lines = append(lines, f.renderPicker(innerW, createCtx{height: screenH}.pickerRows(), sessions)...)
		lines = append(lines, "")
		if f.newWorktree() {
			lines = append(lines, textLine(fieldWorktree, "Worktree", f.worktree, innerW))
		}
		lines = append(lines, textLine(fieldName, "Name", f.name, innerW), textLine(fieldPrompt, "Prompt", f.prompt, innerW))
		if f.prefill.Ticket != "" {
			lines = append(lines, agentstyle.Fit(ui.StyleMuted.Render("  Ticket")+"    "+agentstyle.Ticket.Render(f.prefill.Ticket), innerW))
		}
	}
	hint := createHint
	switch {
	case f.err != "":
		hint = agentstyle.Crashed.Render(f.err)
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
func (f *createForm) renderPicker(innerW, pickerRows int, sessions []wire.SessionInfo) []string {
	rows := f.rows()
	if len(rows) == 0 {
		return []string{ui.StyleMuted.Render("    no matches")}
	}
	end := min(len(rows), f.offset+pickerRows)
	var lines []string
	for _, r := range rows[f.offset:end] {
		if r.entry < 0 {
			lines = append(lines, agentstyle.Fit("  "+agentstyle.ProjectHeader.Render(f.projects[r.project].Name), innerW))
			continue
		}
		e := f.matches[r.entry]
		text := "    " + fuzzy.HighlightMatches(entryLabel(f.home, e), e.pos)
		if !e.newWorktree() {
			if n := dirSessions(sessions, e.dir); n > 0 {
				text += ui.StyleMuted.Render("  " + plural(n, "session"))
			}
		}
		row := agentstyle.Fit(text, innerW)
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
