package chatdrafts

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets/ticketgit"
)

// Workspaces is the part of the workspace store a chat draft needs. The
// workspaces package may import this one, so it is not imported here.
type Workspaces interface {
	WorkspaceOfSession(id string) string
	SetMember(key, workspace string) error
	Remove(key string) error
	// Workspace is the workspace a member key belongs to.
	Workspace(key string) string
}

// Deps is what ChatDrafts is built from.
type Deps struct {
	Log         *slog.Logger
	StateDir    string
	TicketStore string
	Events      *broker.Broker
	Projects    *projects.Registry
	Workspaces  Workspaces
	// Sessions lists the sessions, to find a draft's source session.
	Sessions func() ([]wire.SessionInfo, error)
	// CreateChat is Manager.CreateWebSession and AddWorktree is
	// Manager.AddSessionWorktree, as fields so sessions need not be built in tests.
	CreateChat  func(sessions.WebCreateSessionRequest) (wire.SessionResult, error)
	AddWorktree func(project, name string) (path, refusal string)
	// ForkHookDir is the test-hook dir (see HookArmed); empty turns the hooks off.
	ForkHookDir string
	// History is the Draft history store, shared with the other draft kinds.
	History *drafthistory.Store
}

// ChatDrafts serves the chat draft routes.
type ChatDrafts struct {
	Deps
	store   *Store
	history *drafthistory.Store
}

// NewChatDrafts builds ChatDrafts from d.
func NewChatDrafts(d Deps) *ChatDrafts {
	return &ChatDrafts{Deps: d, store: New(d.StateDir), history: d.History}
}

func refuse(reason, msg string) ops.ChatDraftResult {
	return ops.ChatDraftResult{Refused: true, Reason: reason, Message: msg}
}

func (c *ChatDrafts) publish() {
	if c.Events != nil {
		c.Events.Publish(wire.EventChatDraftsChanged, "")
	}
}

func serverError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func decode[T any](w http.ResponseWriter, r *http.Request) (req T, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func (c *ChatDrafts) create(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftCreateRequest](w, r)
	if !ok {
		return
	}
	if HookArmed(c.ForkHookDir, HookSaveFail) {
		serverError(w, ErrHookSaveFail)
		return
	}
	d, res, err := c.build(req)
	if err != nil {
		serverError(w, err)
		return
	}
	if res.Refused {
		wire.WriteJSON(w, res)
		return
	}
	if err := c.store.Create(d); err != nil {
		serverError(w, err)
		return
	}
	if c.Workspaces != nil {
		// The source's workspace wins over the requested name, as for a Design.
		// An unknown name is refused by SetMember and the draft is rolled back.
		ws := req.Workspace
		if req.FromSession != "" || ws == "" {
			ws = c.Workspaces.WorkspaceOfSession(req.FromSession)
		}
		if err := c.Workspaces.SetMember(MemberKey(d.ID), ws); err != nil {
			if _, delErr := c.store.Delete(d.ID); delErr != nil {
				c.Log.Warn("delete chat draft after membership failed", "draft", d.ID, "err", delErr)
			}
			serverError(w, err)
			return
		}
	}
	c.publish()
	wire.WriteJSON(w, ops.ChatDraftResult{Draft: d})
}

// build validates req and makes the record to store. A refusal is the result.
func (c *ChatDrafts) build(req ops.ChatDraftCreateRequest) (*ops.ChatDraft, ops.ChatDraftResult, error) {
	brief, bad := wire.ValidatePromptText(req.Brief, "the brief")
	if bad != nil {
		return nil, refuse(bad.Reason, bad.Message), nil
	}
	id, err := NewID()
	if err != nil {
		return nil, ops.ChatDraftResult{}, err
	}
	d := &ops.ChatDraft{
		ID: id, Project: req.Project, Name: strings.TrimSpace(req.Name), Harness: req.Harness,
		Model: req.Model, Effort: req.Effort, Brief: brief, Folder: req.Folder, Images: []string{},
	}
	if res := c.checkSettings(d); res != nil {
		return nil, *res, nil
	}
	if res := c.resolveFolder(d, req.FromSession, req.Worktree); res.Refused {
		return nil, res, nil
	}
	if req.FromSession != "" {
		// Resolved on a copy so the source folder is kept whatever folder was chosen.
		probe := *d
		probe.Folder = ops.ChatDraftFolderSource
		if res := c.resolveFolder(&probe, req.FromSession, ""); !res.Refused {
			d.SourceDir = probe.Dir
		}
	}
	return d, ops.ChatDraftResult{}, nil
}

// checkSettings refuses a bad project, harness or effort and gives an empty
// harness the project's launch harness. The model and effort stay as given:
// empty means the harness default, as on the New chat form.
func (c *ChatDrafts) checkSettings(d *ops.ChatDraft) *ops.ChatDraftResult {
	if _, err := c.Projects.Dir(d.Project); err != nil {
		r := refuse(wire.ReasonSessionBadRequest, err.Error())
		return &r
	}
	if d.Harness == "" {
		d.Harness = c.Projects.LaunchHarness(d.Project)
	}
	if _, err := harness.Lookup(agentrunner.Kind(d.Harness)); err != nil {
		r := refuse(wire.ReasonSessionUnknownHarness, err.Error())
		return &r
	}
	if d.Effort != "" && d.Model == "" {
		r := refuse(wire.ReasonSessionBadRequest, "effort needs a model")
		return &r
	}
	return nil
}

// projectRoot is the project's repo root, the folder CheckProjectDir accepts.
func (c *ChatDrafts) projectRoot(project string) string {
	list, err := c.Projects.ListWorktrees()
	if err != nil {
		return ""
	}
	for _, p := range list {
		if p.Project == project {
			return p.Root
		}
	}
	return ""
}

// projectHasDir says whether dir is the project's root or one of its worktrees.
func (c *ChatDrafts) projectHasDir(project, dir string) bool {
	if dir == "" {
		return false
	}
	list, err := c.Projects.ListWorktrees()
	if err != nil {
		return false
	}
	dir = filepath.Clean(dir)
	for _, p := range list {
		if p.Project != project {
			continue
		}
		if filepath.Clean(p.Root) == dir {
			return true
		}
		for _, w := range p.Worktrees {
			if filepath.Clean(w) == dir {
				return true
			}
		}
	}
	return false
}

// resolveFolder fills d.Dir or d.Worktree from the folder choice. It creates
// nothing on disk. A source folder with no fromSession is the project root.
func (c *ChatDrafts) resolveFolder(d *ops.ChatDraft, fromSession, worktree string) ops.ChatDraftResult {
	switch d.Folder {
	case ops.ChatDraftFolderTrunk:
		d.Dir = ticketgit.TrunkCheckout(c.TicketStore, d.Project)
		if d.Dir == "" {
			return refuse(wire.ReasonSessionBadRequest, "project "+d.Project+" has no trunk checkout")
		}
	case ops.ChatDraftFolderSource:
		if fromSession == "" && d.SourceDir != "" {
			d.Dir = d.SourceDir
			break
		}
		if fromSession == "" {
			d.Dir = c.projectRoot(d.Project)
			if d.Dir == "" {
				return refuse(wire.ReasonSessionBadRequest, "project "+d.Project+" has no root folder")
			}
			break
		}
		src, found := c.source(fromSession)
		if !found {
			return refuse(wire.ReasonSessionBadRequest, "the source session was not found")
		}
		d.Dir = src.Dir
		if src.Design {
			// A Design's own worktree is never the folder of a chat drafted from it.
			d.Dir = ticketgit.TrunkCheckout(c.TicketStore, d.Project)
		}
		if d.Dir == "" {
			return refuse(wire.ReasonSessionBadRequest, "the source session has no folder")
		}
	case ops.ChatDraftFolderDir:
		if !c.projectHasDir(d.Project, d.Dir) {
			return refuse(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not a folder of project %s", d.Dir, d.Project))
		}
	case ops.ChatDraftFolderWorktree:
		d.Dir = ""
		d.Worktree = worktree
		if d.Worktree == "" {
			d.Worktree = defaultWorktree(d.Name, d.ID)
		}
		if !filepath.IsLocal(d.Worktree) {
			return refuse(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not a local worktree name", d.Worktree))
		}
	default:
		return refuse(wire.ReasonSessionBadRequest, fmt.Sprintf("folder %q is not source, trunk, dir or worktree", d.Folder))
	}
	return ops.ChatDraftResult{}
}

func (c *ChatDrafts) source(id string) (wire.SessionInfo, bool) {
	if id == "" || c.Sessions == nil {
		return wire.SessionInfo{}, false
	}
	list, err := c.Sessions()
	if err != nil {
		return wire.SessionInfo{}, false
	}
	for _, s := range list {
		if s.ID == id {
			return s, true
		}
	}
	return wire.SessionInfo{}, false
}

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// defaultWorktree is "<slug of name>-<first 6 hex of id>", "chat-..." for a name with no slug.
func defaultWorktree(name, id string) string {
	slug := strings.Trim(nonSlugRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = "chat"
	}
	return slug + "-" + id[:6]
}

func (c *ChatDrafts) read(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftReadRequest](w, r)
	if !ok {
		return
	}
	d, err := c.store.Get(req.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		wire.WriteJSON(w, c.gone(req.ID))
	case err != nil:
		serverError(w, err)
	default:
		wire.WriteJSON(w, ops.ChatDraftResult{Draft: d})
	}
}

func (c *ChatDrafts) save(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftSaveRequest](w, r)
	if !ok {
		return
	}
	res, err := c.store.SaveChecked(req, c.checkSave)
	if err != nil {
		serverError(w, err)
		return
	}
	// The sidebar label can change with the name; a brief-only save changes no row.
	if !res.Refused && req.Settings() {
		c.publish()
	}
	wire.WriteJSON(w, res)
}

// checkSave validates the merged draft of a save and re-resolves its folder
// when the project or the folder choice changed.
func (c *ChatDrafts) checkSave(d *ops.ChatDraft, before ops.ChatDraft) *ops.ChatDraftResult {
	if d.Project == before.Project && d.Harness == before.Harness && d.Model == before.Model &&
		d.Effort == before.Effort && d.Folder == before.Folder && d.Worktree == before.Worktree &&
		d.Dir == before.Dir {
		return nil
	}
	if res := c.checkSettings(d); res != nil {
		return res
	}
	if d.Project != before.Project {
		d.SourceDir = "" // the source chat's folder belongs to the old project
	}
	if d.Folder == ops.ChatDraftFolderWorktree || d.Project != before.Project || d.Folder != before.Folder ||
		d.Dir != before.Dir {
		if res := c.resolveFolder(d, "", d.Worktree); res.Refused {
			return &res
		}
	}
	return nil
}

// deleteDraft is the explicit delete: read, save to history and remove under
// one hold of the draft's lock. A failed save refuses the delete and keeps the
// draft, so the text is never lost without a copy.
func (c *ChatDrafts) deleteDraft(id string) (ops.ChatDraftResult, error) {
	unlock := c.store.lock(id)
	defer unlock()
	d, err := c.store.Get(id)
	if errors.Is(err, ErrNotFound) {
		return ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	if err != nil {
		return ops.ChatDraftResult{}, err
	}
	if err := c.saveHistory(d); err != nil {
		return ops.ChatDraftResult{}, err
	}
	if _, err := c.store.remove(id); err != nil {
		return ops.ChatDraftResult{}, err
	}
	return ops.ChatDraftResult{Deleted: true}, nil
}

func (c *ChatDrafts) remove(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftReadRequest](w, r)
	if !ok {
		return
	}
	res, err := c.deleteDraft(req.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if res.Refused {
		wire.WriteJSON(w, res)
		return
	}
	if c.Workspaces != nil {
		if err := c.Workspaces.Remove(MemberKey(req.ID)); err != nil {
			c.Log.Warn("drop chat draft membership", "draft", req.ID, "err", err)
		}
	}
	c.publish()
	wire.WriteJSON(w, ops.ChatDraftResult{Deleted: true})
}

// Routes lists the chat draft routes. Create, delete and the image
// download have a socket handler; the rest are web-only. The getter is called per request, so a route can be listed before its
// ChatDrafts is built.
func Routes(get func() *ChatDrafts) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/agents/drafts/create", Web: wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().create(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().create(w, r) }},
		{Pattern: "POST /v1/agents/drafts/read", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().read(w, r) }},
		{Pattern: "POST /v1/agents/drafts/save", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().save(w, r) }},
		{Pattern: "POST /v1/agents/drafts/delete", Web: wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().remove(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().remove(w, r) }},
		{Pattern: "POST /v1/agents/drafts/start", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().start(w, r) }},
		{Pattern: "POST /v1/agents/drafts/pick-history", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().pickHistory(w, r) }},
		{Pattern: "POST /v1/agents/drafts/images", Web: wire.WebWrite, RawBody: true,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().imageUpload(w, r) }},
		{Pattern: "GET /v1/agents/drafts/images", Web: wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().imageServe(w, r) }},
		{Pattern: "POST /v1/agents/drafts/images/delete", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().imageRemove(w, r) }},
	}
}
