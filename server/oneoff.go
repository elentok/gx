package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// Refusal reasons of a one-off submit.
const (
	ReasonEmptyPrompt   = "empty-prompt"
	ReasonUnknownProj   = "unknown-project"
	ReasonNoCommits     = "commits-unsupported"
	ReasonBadBase       = "invalid-base"
	ReasonBadBlocker    = "invalid-blocker"
	ReasonBadWindow     = "invalid-context-window"
	ReasonBadFile       = "invalid-file"
	ReasonDuplicateLive = "duplicate-live"
)

// OneOffStatusQueued is the status a successful submit reports: it is created
// and already at the queue tail.
const OneOffStatusQueued = "queued"

// oneOffNameWords is how many of the prompt's first words seed a default name.
const oneOffNameWords = 4

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// OneOffRequest is the body of a submit. Only Prompt is required.
type OneOffRequest struct {
	Prompt string `json:"prompt"`
	// Project wins over Cwd; with neither matching, the ticket lands in scratch.
	Project string `json:"project,omitempty"`
	// Cwd is the caller's directory, resolved to the registered project owning it.
	Cwd   string `json:"cwd,omitempty"`
	Name  string `json:"name,omitempty"`
	Agent string `json:"agent,omitempty"`
	// Commits makes the ticket type implement (prompt otherwise); a project without a VCS refuses it.
	Commits bool `json:"commits,omitempty"`
	// Base overrides the derived base; only a commit-landing ticket has one.
	Base string `json:"base,omitempty"`
	// BlockedBy are addresses of tickets in the same project.
	BlockedBy             []string `json:"blocked_by,omitempty"`
	ExpectedContextWindow int      `json:"expected_context_window,omitempty"`
	// Front queues it at the head instead of the tail.
	Front bool `json:"front,omitempty"`
	// Notify sends the ticket's Result to chat on success; parks always notify.
	Notify bool `json:"notify,omitempty"`
	// File is a server-side path to a markdown payload (frontmatter + body).
	// Its frontmatter fills every field left unset here; the body is the prompt
	// when Prompt is empty.
	File string `json:"file,omitempty"`
	// Unique is the dedupe key; a --file submit defaults it to the file's
	// resolved path unless NoUnique is set. A plain prompt has none.
	Unique   string `json:"unique,omitempty"`
	NoUnique bool   `json:"no_unique,omitempty"`
}

// oneOffFileFrontmatter is the subset of a payload file's frontmatter a submit
// understands.
type oneOffFileFrontmatter struct {
	Project               string   `yaml:"project"`
	Name                  string   `yaml:"name"`
	Agent                 string   `yaml:"agent"`
	Commits               bool     `yaml:"commits"`
	Base                  string   `yaml:"base"`
	BlockedBy             []string `yaml:"blocked_by"`
	ExpectedContextWindow int      `yaml:"expected_context_window"`
	Front                 bool     `yaml:"front"`
	Unique                string   `yaml:"unique"`
}

// applyFile merges the payload file into req; anything already set on req wins.
func (req *OneOffRequest) applyFile() error {
	raw, err := os.ReadFile(req.File)
	if err != nil {
		return err
	}
	yamlPart, body, hasFM := schema.SplitFrontmatter(string(raw))
	var fm oneOffFileFrontmatter
	if hasFM {
		if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
			return err
		}
	} else {
		body = string(raw)
	}
	setIfEmpty := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	setIfEmpty(&req.Prompt, body)
	setIfEmpty(&req.Project, fm.Project)
	setIfEmpty(&req.Name, fm.Name)
	setIfEmpty(&req.Agent, fm.Agent)
	setIfEmpty(&req.Base, fm.Base)
	setIfEmpty(&req.Unique, fm.Unique)
	req.Commits = req.Commits || fm.Commits
	req.Front = req.Front || fm.Front
	if len(req.BlockedBy) == 0 {
		req.BlockedBy = fm.BlockedBy
	}
	if req.ExpectedContextWindow == 0 {
		req.ExpectedContextWindow = fm.ExpectedContextWindow
	}
	return nil
}

// OneOffResult is a submit's answer; a refusal creates nothing.
type OneOffResult struct {
	Address string `json:"address,omitempty"`
	Status  string `json:"status,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func oneOffRefusal(reason, msg string) OneOffResult {
	return OneOffResult{Refused: true, Reason: reason, Message: msg}
}

// oneOffPlan is a fully validated submit, ready to write.
type oneOffPlan struct {
	project, dir, epic, prompt string
	addr                       tickets.Address
	tk                         schema.Ticket
	agent                      string
	front                      bool
}

// oneOff creates a top-level ticket carrying the prompt and appends it to the
// queue in one call. Everything that can refuse is checked before the first
// write, and a refusal after the write removes the ticket again, so a refused
// submit leaves nothing behind for a retry to collide with.
func (s *Server) oneOff(req OneOffRequest) (OneOffResult, error) {
	var plan oneOffPlan
	if res, err := s.oneOffValidate(req, &plan); err != nil || res.Refused {
		return res, err
	}
	if res, err := oneOffWrite(plan); err != nil || res.Refused {
		return res, err
	}
	return s.oneOffEnqueue(plan)
}

// oneOffValidate checks the submit and fills plan. A refusal (or error) means
// plan is unusable; nothing has been written.
func (s *Server) oneOffValidate(req OneOffRequest, plan *oneOffPlan) (OneOffResult, error) {
	if req.File != "" {
		if err := req.applyFile(); err != nil {
			return oneOffRefusal(ReasonBadFile, err.Error()), nil
		}
		if req.Unique == "" && !req.NoUnique {
			resolved, err := filepath.EvalSymlinks(req.File)
			if err != nil {
				return oneOffRefusal(ReasonBadFile, err.Error()), nil
			}
			if req.Unique, err = filepath.Abs(resolved); err != nil {
				return oneOffRefusal(ReasonBadFile, err.Error()), nil
			}
		}
	}
	if req.NoUnique {
		req.Unique = ""
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return oneOffRefusal(ReasonEmptyPrompt, "a one-off needs a prompt"), nil
	}
	typ := schema.TypePrompt
	if req.Commits {
		typ = schema.TypeImplement
	}
	project, dir, err := s.oneOffProject(req)
	if err != nil {
		return OneOffResult{}, err
	}
	if dir == "" {
		return oneOffRefusal(ReasonUnknownProj, "no project "+project+" in the ticket store"), nil
	}

	epic := req.Name
	if epic == "" {
		epic = defaultOneOffName(prompt)
	}
	addr := tickets.Address{Project: project, Epic: epic, ID: "01"}
	if _, err := tickets.ParseAddress(addr.String(), tickets.AddressContext{}); err != nil {
		return oneOffRefusal(ReasonInvalidAddress, "name "+epic+" is not usable in a ticket address"), nil
	}
	if dup, err := s.liveDuplicate(dir, req.Unique); err != nil {
		return OneOffResult{}, err
	} else if dup != nil {
		return dup.refusal(project, dir)
	}
	tk := schema.Ticket{ID: "01", Status: schema.StatusOpen, Type: typ, Base: req.Base, Notify: req.Notify, Unique: req.Unique, ExpectedContextWindow: req.ExpectedContextWindow}
	if refused := s.oneOffOptionsRefusal(project, dir, tk, req); refused != nil {
		return *refused, nil
	}
	for _, b := range req.BlockedBy {
		a, _ := tickets.ParseAddress(b, tickets.AddressContext{})
		tk.BlockedBy = append(tk.BlockedBy, schema.TicketID(a.Epic+"/"+a.ID))
	}
	// The queue refuses an unknown agent; the new epic can be neither a map epic
	// nor already queued, so the agent is the only enqueue check left to do here.
	agent, bad := resolveAgent(req.Agent)
	if bad != nil {
		return oneOffRefusal(bad.Reason, bad.Message), nil
	}
	*plan = oneOffPlan{project: project, dir: dir, epic: epic, prompt: prompt, addr: addr, tk: tk, agent: string(agent), front: req.Front}
	return OneOffResult{}, nil
}

// oneOffWrite puts the ticket on disk.
func oneOffWrite(p oneOffPlan) (OneOffResult, error) {
	if err := writeOneOffTicket(p.dir, p.epic, p.tk, p.prompt); err != nil {
		if os.IsExist(err) {
			return oneOffRefusal(ReasonNameTaken, p.project+":"+p.epic+" already exists"), nil
		}
		return OneOffResult{}, err
	}
	return OneOffResult{}, nil
}

// oneOffEnqueue queues the written ticket and only then records `submitted`.
// Any failure removes the epic dir again so nothing unqueued stays behind.
func (s *Server) oneOffEnqueue(p oneOffPlan) (OneOffResult, error) {
	res, err := s.oneOffQueue(p)
	if err != nil || res.Refused {
		if rmErr := os.RemoveAll(filepath.Join(p.dir, p.epic)); rmErr != nil {
			s.log.Warn("remove unqueued one-off", "ticket", p.addr.String(), "err", rmErr)
		}
		if refreshErr := s.idx.refresh(s.cfg.TicketStore); refreshErr != nil {
			s.log.Warn("refresh after one-off cleanup", "err", refreshErr)
		}
		return res, err
	}
	if err := ralphloop.AppendEvent(p.dir, p.epic, ralphloop.Event{
		Time: time.Now(), Type: string(events.Submitted), Ticket: p.addr.ID, Address: p.addr.String(),
	}); err != nil {
		return OneOffResult{}, err
	}
	return res, nil
}

func (s *Server) oneOffQueue(p oneOffPlan) (OneOffResult, error) {
	// queueAdd checks the index, so it must see the new ticket first.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		return OneOffResult{}, err
	}
	q, err := s.queueAdd(QueueRequest{Address: p.addr.String(), Agent: p.agent, Front: p.front})
	if err != nil {
		return OneOffResult{}, err
	}
	if q.Refused {
		return oneOffRefusal(q.Reason, q.Message), nil
	}
	return OneOffResult{Address: p.addr.String(), Status: OneOffStatusQueued}, nil
}

// duplicate is the live ticket a submit's dedupe key collides with.
type duplicate struct {
	epic, id string
}

// liveDuplicate finds a ticket of the project carrying key that is not done or
// cancelled; parked copies count as live. Reads the files, not the index, so a
// just-written ticket is seen.
func (s *Server) liveDuplicate(projectDir, key string) (*duplicate, error) {
	if key == "" {
		return nil, nil
	}
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return nil, err
	}
	for _, e := range epics {
		for _, t := range e.Tickets {
			terminal := t.Status == string(schema.StatusDone) || t.Status == string(schema.StatusCancelled)
			if t.Unique == key && !terminal {
				return &duplicate{epic: filepath.Base(e.Path), id: t.DisplayNumber()}, nil
			}
		}
	}
	return nil, nil
}

// refusal records the refusal in the duplicate's epic log (a refused submit
// has no epic of its own) and answers with the duplicate's address.
func (d *duplicate) refusal(project, projectDir string) (OneOffResult, error) {
	addr := tickets.Address{Project: project, Epic: d.epic, ID: d.id}.String()
	if err := ralphloop.AppendEvent(projectDir, d.epic, ralphloop.Event{
		Time: time.Now(), Type: string(events.SubmitRefused), Ticket: d.id, Address: addr,
		Kind: string(events.DuplicateLive), Reason: "a live ticket already has this dedupe key",
	}); err != nil {
		return OneOffResult{}, err
	}
	res := oneOffRefusal(ReasonDuplicateLive, addr+" already has this dedupe key")
	res.Address = addr
	return res, nil
}

// notifyResult sends a landed ticket's ## Result to chat when its submit asked
// for it. A ticket without the flag stays silent; parks never reach here.
func (s *Server) notifyResult(addr tickets.Address, ticketPath string) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || !tk.Notify {
		return
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		s.log.Warn("read result", "ticket", addr.String(), "err", err)
		return
	}
	dir := filepath.Dir(filepath.Dir(filepath.Dir(ticketPath))) // project/epic/issues/file
	s.chat.Result(addr.Project, s.chatOverride(addr.Project), dir, addr.Epic, addr.String(), schema.Section(schema.ParseBody(string(raw)), "Result"))
}

// oneOffOptionsRefusal checks the submit options before anything is written. A
// blocker must be a ticket of the same project (no cross-project blocking).
func (s *Server) oneOffOptionsRefusal(project, dir string, tk schema.Ticket, req OneOffRequest) *OneOffResult {
	refuse := func(reason, msg string) *OneOffResult {
		r := oneOffRefusal(reason, msg)
		return &r
	}
	if req.Commits {
		pf, err := config.ReadProjectFile(dir)
		if err == nil && pf.VCS != nil && *pf.VCS == config.VCSNone {
			return refuse(ReasonNoCommits, project+" has no VCS, so its tickets never commit")
		}
	}
	if req.ExpectedContextWindow < 0 {
		return refuse(ReasonBadWindow, "expected context window must be non-negative")
	}
	if req.Base != "" && tk.IsCommitless() {
		return refuse(ReasonBadBase, "--base needs --commits: a commitless ticket has no branch to base")
	}
	if len(req.BlockedBy) == 0 {
		return nil
	}
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		r := oneOffRefusal(ReasonBadBlocker, err.Error())
		return &r
	}
	for _, b := range req.BlockedBy {
		a, err := tickets.ParseAddress(b, tickets.AddressContext{})
		switch {
		case err != nil:
			return refuse(ReasonBadBlocker, err.Error())
		case a.Project != project:
			return refuse(ReasonBadBlocker, b+" is in another project (cross-project blocking not supported)")
		case !s.hasTicket(a.String()):
			return refuse(ReasonBadBlocker, "no ticket "+b)
		}
	}
	return nil
}

// oneOffProject picks the project: the explicit one, else the registered
// project owning Cwd, else scratch. dir is empty when the project is unknown.
func (s *Server) oneOffProject(req OneOffRequest) (project, dir string, err error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return "", "", err
	}
	if req.Project != "" {
		for _, d := range dirs {
			if tickets.ProjectName(d) == req.Project {
				return req.Project, d, nil
			}
		}
		return req.Project, "", nil
	}
	cwd := canonicalDir(req.Cwd)
	best, bestLen := ScratchProject, -1
	for _, d := range dirs {
		name := tickets.ProjectName(d)
		pf, err := config.ReadProjectFile(d)
		if name == ScratchProject || err != nil || pf.Repo == nil {
			continue
		}
		for _, root := range projectRoots(canonicalDir(*pf.Repo)) {
			if req.Cwd != "" && within(root, cwd) && len(root) > bestLen {
				best, bestLen = name, len(root)
			}
		}
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) == best {
			return best, d, nil
		}
	}
	return best, "", nil
}

// projectRoots are the directories whose subtree belongs to a project: its
// repo, and for a bare repo (.../x/.bare) the directory holding its worktrees.
func projectRoots(repo string) []string {
	if filepath.Base(repo) == ".bare" {
		return []string{filepath.Dir(repo)}
	}
	return []string{repo}
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// defaultOneOffName is the prompt's first words plus a short random suffix, so
// two submits of the same prompt never collide.
func defaultOneOffName(prompt string) string {
	words := strings.Fields(strings.ToLower(prompt))
	if len(words) > oneOffNameWords {
		words = words[:oneOffNameWords]
	}
	slug := strings.Trim(nonSlugRe.ReplaceAllString(strings.Join(words, "-"), "-"), "-")
	if slug == "" {
		slug = "one-off"
	}
	var b [3]byte
	_, _ = rand.Read(b[:])
	return slug + "-" + hex.EncodeToString(b[:])
}

// oneOffFile is the name of a one-off's single ticket; nothing else writes it.
const oneOffFile = "01-one-off.md"

// isOneOffTicket reports whether ticketPath is a one-off's ticket.
func isOneOffTicket(ticketPath string) bool {
	return filepath.Base(ticketPath) == oneOffFile
}

// writeOneOffTicket creates the epic directory and its single ticket. The
// directory creation is the uniqueness check: an existing epic is os.ErrExist.
func writeOneOffTicket(projectDir, epic string, tk schema.Ticket, prompt string) error {
	epicDir := filepath.Join(projectDir, epic)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(epicDir, 0o755); err != nil {
		return err
	}
	issues := filepath.Join(epicDir, "issues")
	if err := os.Mkdir(issues, 0o755); err != nil {
		return err
	}
	if err := tickets.WriteEpicTicketMD(epicDir, schema.StatusOpen); err != nil {
		return err
	}
	out, err := schema.MarshalTicket(tk, "\n"+prompt+"\n")
	if err != nil {
		return err
	}
	path := filepath.Join(issues, oneOffFile)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	_, err = schema.ParseTicket(path)
	return err
}

func (s *Server) oneOffHandler(w http.ResponseWriter, r *http.Request) {
	var req OneOffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.oneOff(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
