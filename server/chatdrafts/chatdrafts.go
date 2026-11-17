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
}

// ChatDrafts serves the chat draft routes.
type ChatDrafts struct {
	Deps
	store *Store
}

// NewChatDrafts builds ChatDrafts from d.
func NewChatDrafts(d Deps) *ChatDrafts {
	return &ChatDrafts{Deps: d, store: New(d.StateDir)}
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
		ws := c.Workspaces.WorkspaceOfSession(req.FromSession)
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
	switch {
	case bad != nil:
		return nil, refuse(bad.Reason, bad.Message), nil
	case brief == "":
		return nil, refuse(wire.ReasonEmptyPrompt, "a brief is required"), nil
	}
	if _, err := c.Projects.Dir(req.Project); err != nil {
		return nil, refuse(wire.ReasonSessionBadRequest, err.Error()), nil
	}
	hn := req.Harness
	if hn == "" {
		hn = c.Projects.LaunchHarness(req.Project)
	}
	if _, err := harness.Lookup(agentrunner.Kind(hn)); err != nil {
		return nil, refuse(wire.ReasonSessionUnknownHarness, err.Error()), nil
	}
	if req.Effort != "" && req.Model == "" {
		return nil, refuse(wire.ReasonSessionBadRequest, "effort needs a model"), nil
	}
	id, err := NewID()
	if err != nil {
		return nil, ops.ChatDraftResult{}, err
	}
	d := &ops.ChatDraft{
		ID: id, Project: req.Project, Name: strings.TrimSpace(req.Name), Harness: hn,
		Model: req.Model, Effort: req.Effort, Brief: brief, Folder: req.Folder,
	}
	if res := c.resolveFolder(d, req); res.Refused {
		return nil, res, nil
	}
	return d, ops.ChatDraftResult{}, nil
}

// resolveFolder fills d.Dir or d.Worktree from the folder choice. It creates
// nothing on disk.
func (c *ChatDrafts) resolveFolder(d *ops.ChatDraft, req ops.ChatDraftCreateRequest) ops.ChatDraftResult {
	switch d.Folder {
	case ops.ChatDraftFolderTrunk:
		d.Dir = ticketgit.TrunkCheckout(c.TicketStore, d.Project)
		if d.Dir == "" {
			return refuse(wire.ReasonSessionBadRequest, "project "+d.Project+" has no trunk checkout")
		}
	case ops.ChatDraftFolderSource:
		src, found := c.source(req.FromSession)
		if !found {
			return refuse(wire.ReasonSessionBadRequest, "a source folder needs a source session")
		}
		d.Dir = src.Dir
		if src.Design {
			// A Design's own worktree is never the folder of a chat drafted from it.
			d.Dir = ticketgit.TrunkCheckout(c.TicketStore, d.Project)
		}
		if d.Dir == "" {
			return refuse(wire.ReasonSessionBadRequest, "the source session has no folder")
		}
	case ops.ChatDraftFolderWorktree:
		d.Worktree = req.Worktree
		if d.Worktree == "" {
			d.Worktree = defaultWorktree(d.Name, d.ID)
		}
		if !filepath.IsLocal(d.Worktree) {
			return refuse(wire.ReasonSessionBadRequest, fmt.Sprintf("%q is not a local worktree name", d.Worktree))
		}
	default:
		return refuse(wire.ReasonSessionBadRequest, fmt.Sprintf("folder %q is not source, trunk or worktree", d.Folder))
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
	res, err := c.store.Save(req)
	if err != nil {
		serverError(w, err)
		return
	}
	wire.WriteJSON(w, res)
}

func (c *ChatDrafts) remove(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[ops.ChatDraftReadRequest](w, r)
	if !ok {
		return
	}
	existed, err := c.store.Delete(req.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !existed {
		wire.WriteJSON(w, ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftGone})
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

// Routes lists the chat draft routes. All are web-only: the CLI verbs arrive
// later. The getter is called per request, so a route can be listed before its
// ChatDrafts is built.
func Routes(get func() *ChatDrafts) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/agents/drafts/create", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().create(w, r) }},
		{Pattern: "POST /v1/agents/drafts/read", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().read(w, r) }},
		{Pattern: "POST /v1/agents/drafts/save", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().save(w, r) }},
		{Pattern: "POST /v1/agents/drafts/delete", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().remove(w, r) }},
		{Pattern: "POST /v1/agents/drafts/start", Web: wire.WebWrite,
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().start(w, r) }},
	}
}
