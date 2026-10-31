package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	ReasonOptionRemoved = "option-removed"
	ReasonBadBlocker    = "invalid-blocker"
	ReasonBadWindow     = "invalid-context-window"
	ReasonBadFile       = "invalid-file"
	ReasonDuplicateLive = "duplicate-live"
	ReasonBadType       = "invalid-type"
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
	// Project wins over Cwd; with neither matching, the ticket merges in scratch.
	Project string `json:"project,omitempty"`
	// Cwd is the caller's directory, resolved to the registered project owning it.
	Cwd   string `json:"cwd,omitempty"`
	Name  string `json:"name,omitempty"`
	Agent string `json:"agent,omitempty"`
	// Type is the ticket type: "prompt" (default), "evolve" or "diagnose". A
	// one-off never commits.
	Type string `json:"type,omitempty"`
	// BlockedBy are addresses of tickets in the same project.
	BlockedBy             []string `json:"blocked_by,omitempty"`
	ExpectedContextWindow int      `json:"expected_context_window,omitempty"`
	// Front queues it at the head instead of the tail.
	Front bool `json:"front,omitempty"`
	// Notify sends the ticket's Result to chat on success; pauses always notify.
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

// RetroSubmit is the one-off a retro of epic runs as: a commitless evolve ticket
// in the epic's own project. The dedupe key refuses a second live retro of the
// same epic; the timestamped name keeps a --force re-run from hitting a taken one.
func RetroSubmit(project, epic string, now time.Time) OneOffRequest {
	return OneOffRequest{
		Prompt:  "Retro of epic " + project + ":" + epic,
		Project: project,
		Name:    "evolve-" + epic + "-" + now.Format("20060102-1504"),
		Type:    string(schema.TypeEvolve),
		Unique:  "evolve:" + project + "/" + epic,
		Notify:  true,
	}
}

// oneOffFileFrontmatter is the subset of a payload file's frontmatter a submit
// understands.
type oneOffFileFrontmatter struct {
	Project               string   `yaml:"project"`
	Name                  string   `yaml:"name"`
	Agent                 string   `yaml:"agent"`
	BlockedBy             []string `yaml:"blocked_by"`
	ExpectedContextWindow int      `yaml:"expected_context_window"`
	Front                 bool     `yaml:"front"`
	Unique                string   `yaml:"unique"`
}

// removedFileKeys are payload keys one-offs no longer accept.
var removedFileKeys = []string{"commits", "base"}

// removedKeyError is a payload frontmatter key one-offs no longer accept.
type removedKeyError struct{ key string }

func (e removedKeyError) Error() string {
	return "the " + e.key + " key is no longer supported: a one-off never commits"
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
		var keys map[string]any
		if err := yaml.Unmarshal([]byte(yamlPart), &keys); err != nil {
			return err
		}
		for _, k := range removedFileKeys {
			if _, ok := keys[k]; ok {
				return removedKeyError{k}
			}
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
	setIfEmpty(&req.Unique, fm.Unique)
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
	project, dir, epic, slug, prompt string
	tk                               schema.Ticket
	agent                            string
	front                            bool
}

// writtenOneOff is where a plan's ticket landed on disk.
type writtenOneOff struct {
	addr tickets.Address
	path string
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
	written, err := oneOffWrite(plan)
	if err != nil {
		return OneOffResult{}, err
	}
	return s.oneOffEnqueue(plan, written)
}

// oneOffValidate checks the submit and fills plan. A refusal (or error) means
// plan is unusable; nothing has been written.
func (s *Server) oneOffValidate(req OneOffRequest, plan *oneOffPlan) (OneOffResult, error) {
	if req.File != "" {
		if err := req.applyFile(); err != nil {
			var removed removedKeyError
			if errors.As(err, &removed) {
				return oneOffRefusal(ReasonOptionRemoved, err.Error()), nil
			}
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
	switch schema.TicketType(req.Type) {
	case "", schema.TypePrompt:
	case schema.TypeEvolve, schema.TypeDiagnose:
		typ = schema.TicketType(req.Type)
	default:
		return oneOffRefusal(ReasonBadType, "type "+req.Type+" is not supported by a one-off"), nil
	}
	project, dir, err := s.oneOffProject(req)
	if err != nil {
		return OneOffResult{}, err
	}
	if dir == "" {
		return oneOffRefusal(ReasonUnknownProj, "no project "+project+" in the ticket store"), nil
	}

	slug := req.Name
	if slug == "" {
		slug = defaultOneOffName(prompt)
	}
	if _, err := tickets.ParseAddress(tickets.Address{Project: project, Epic: slug, ID: "01"}.String(), tickets.AddressContext{}); err != nil {
		return oneOffRefusal(ReasonInvalidAddress, "name "+slug+" is not usable in a ticket address"), nil
	}
	if dup, err := s.liveDuplicate(dir, req.Unique); err != nil {
		return OneOffResult{}, err
	} else if dup != nil {
		return dup.refusal(project, dir)
	}
	tk := schema.Ticket{Status: schema.StatusOpen, Type: typ, Notify: req.Notify, Unique: req.Unique, ExpectedContextWindow: req.ExpectedContextWindow}
	if refused := s.oneOffOptionsRefusal(project, req); refused != nil {
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
	tk.Agent = string(agent)
	*plan = oneOffPlan{project: project, dir: dir, epic: OneOffsEpic, slug: slug, prompt: prompt, tk: tk, agent: string(agent), front: req.Front}
	return OneOffResult{}, nil
}

// oneOffWrite puts the ticket on disk in the shared epic.
func oneOffWrite(p oneOffPlan) (writtenOneOff, error) {
	path, id, err := writeOneOffTicket(p.dir, p.slug, p.tk, p.prompt)
	if err != nil {
		return writtenOneOff{}, err
	}
	return writtenOneOff{addr: tickets.Address{Project: p.project, Epic: p.epic, ID: id}, path: path}, nil
}

// oneOffEnqueue queues the written ticket and only then records `submitted`.
// Any failure removes the new ticket file again so nothing unqueued stays
// behind; the shared epic stays.
func (s *Server) oneOffEnqueue(p oneOffPlan, w writtenOneOff) (OneOffResult, error) {
	res, err := s.oneOffQueue(p, w)
	if err != nil || res.Refused {
		if rmErr := os.Remove(w.path); rmErr != nil {
			s.log.Warn("remove unqueued one-off", "ticket", w.addr.String(), "err", rmErr)
		}
		if refreshErr := s.idx.refresh(s.cfg.TicketStore); refreshErr != nil {
			s.log.Warn("refresh after one-off cleanup", "err", refreshErr)
		}
		return res, err
	}
	if err := ralphloop.AppendEvent(p.dir, p.epic, ralphloop.Event{
		Time: time.Now(), Type: string(events.Submitted), Ticket: w.addr.ID, Address: w.addr.String(),
	}); err != nil {
		return OneOffResult{}, err
	}
	return res, nil
}

func (s *Server) oneOffQueue(p oneOffPlan, w writtenOneOff) (OneOffResult, error) {
	// queueAdd checks the index, so it must see the new ticket first.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		return OneOffResult{}, err
	}
	q, err := s.queueRoot(w.addr, p.agent, p.front)
	if err != nil {
		return OneOffResult{}, err
	}
	if q.Refused {
		return oneOffRefusal(q.Reason, q.Message), nil
	}
	return OneOffResult{Address: w.addr.String(), Status: OneOffStatusQueued}, nil
}

// queueRoot queues the root of addr. The shared epic is queued once, as a root:
// a later submit only moves it.
func (s *Server) queueRoot(addr tickets.Address, agent string, front bool) (QueueResult, error) {
	root := rootOf(addr)
	if !s.rootQueued(root) {
		return s.queueAdd(QueueRequest{Address: addr.String(), Agent: agent, Front: front})
	}
	res, err := s.queueRaise(root, front)
	var agents []string
	for _, it := range s.queued.list() {
		if a, perr := tickets.ParseAddress(it.Address, tickets.AddressContext{}); perr == nil && rootOf(a) == root {
			agents = append(agents, it.Agent)
		}
	}
	return s.withExtraUsage(root.Project, agents, res, err)
}

// queueRaise moves every queued item of root to the head, keeping their order.
// Without front the queue is left as it is.
func (s *Server) queueRaise(root rootRef, front bool) (QueueResult, error) {
	return s.writeQueue(root.String(), func(items []QueueItem) ([]QueueItem, *QueueResult) {
		if !front {
			return items, nil
		}
		var head, rest []QueueItem
		for _, it := range items {
			if a, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil && rootOf(a) == root {
				head = append(head, it)
			} else {
				rest = append(rest, it)
			}
		}
		return append(head, rest...), nil
	})
}

// duplicate is the live ticket a submit's dedupe key collides with.
type duplicate struct {
	epic, id string
}

// liveDuplicate finds a ticket of the project carrying key that is not done or
// cancelled; paused copies count as live. Reads the files, not the index, so a
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
		if e.Name == OneOffsArchivedEpic {
			continue
		}
		for _, t := range e.Tickets {
			if t.Unique == key && !tickets.IsTerminalStatus(t.Status) {
				return &duplicate{epic: e.Name, id: t.DisplayNumber()}, nil
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

// notifyResult sends a merged ticket's ## Result to chat when its submit asked
// for it. A ticket without the flag stays silent; pauses never reach here.
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
func (s *Server) oneOffOptionsRefusal(project string, req OneOffRequest) *OneOffResult {
	refuse := func(reason, msg string) *OneOffResult {
		r := oneOffRefusal(reason, msg)
		return &r
	}
	if req.ExpectedContextWindow < 0 {
		return refuse(ReasonBadWindow, "expected context window must be non-negative")
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

// defaultOneOffName is the prompt's first words. The ticket number keeps two
// submits of the same prompt apart.
func defaultOneOffName(prompt string) string {
	words := strings.Fields(strings.ToLower(prompt))
	if len(words) > oneOffNameWords {
		words = words[:oneOffNameWords]
	}
	slug := strings.Trim(nonSlugRe.ReplaceAllString(strings.Join(words, "-"), "-"), "-")
	if slug == "" {
		slug = "one-off"
	}
	return slug
}

// oneOffFile is the name of a one-off's single ticket; nothing else writes it.
const oneOffFile = "01-one-off.md"

// The epics holding one-off tickets: every ticket in OneOffsEpic is a one-off,
// whoever wrote it. OneOffsArchivedEpic is where finished ones are moved.
const (
	OneOffsEpic         = tickets.OneOffsEpic
	OneOffsArchivedEpic = tickets.ArchivedOneOffsEpic
)

func isOneOffsEpicName(name string) bool {
	return name == OneOffsEpic || name == OneOffsArchivedEpic
}

// isOneOffTicket reports whether ticketPath (project/epic/issues/file) is a
// one-off's ticket: in a one-offs epic, or the single ticket of an old
// one-epic-per-one-off submit.
func isOneOffTicket(ticketPath string) bool {
	epic := filepath.Base(filepath.Dir(filepath.Dir(ticketPath)))
	return isOneOffsEpicName(epic) || filepath.Base(ticketPath) == oneOffFile
}

// IsOneOffEpic reports whether e is a one-off epic: one of the one-offs epics,
// or an old single-ticket epic whose ticket is the one writeOneOffTicket wrote.
func IsOneOffEpic(e tickets.Epic) bool {
	if isOneOffsEpicName(filepath.Base(e.Path)) {
		return true
	}
	for _, t := range e.Tickets {
		if isOneOffTicket(t.FilePath()) {
			return true
		}
	}
	return false
}

// nextOneOffNumber is above the highest in the live epic and the archived one.
func nextOneOffNumber(projectDir string, epic *tickets.Epic) (int, error) {
	next := 1
	if id, err := tickets.NextTicketID(*epic, ""); err != nil {
		return 0, err
	} else if n, err := strconv.Atoi(id); err == nil {
		next = n
	}
	_, archived, err := tickets.LoadOneOffs(projectDir)
	if err != nil {
		return 0, err
	}
	for _, t := range archived.Tickets {
		next = max(next, t.Number+1)
	}
	return next, nil
}

// writeOneOffTicket adds the ticket to the shared one-offs epic, creating the
// epic as a draft on the first submit. The id is above the highest in both
// one-offs epics, so an archived number is never reused.
func writeOneOffTicket(projectDir, slug string, tk schema.Ticket, prompt string) (path, id string, err error) {
	epicDir := filepath.Join(projectDir, OneOffsEpic)
	if err := ensureDraftEpic(epicDir); err != nil {
		return "", "", err
	}
	epic, unlock, err := tickets.LoadLockedEpic(epicDir)
	if err != nil {
		return "", "", err
	}
	defer unlock()
	next, err := nextOneOffNumber(projectDir, epic)
	if err != nil {
		return "", "", err
	}
	id = fmt.Sprintf("%02d", next)
	tk.ID = schema.TicketID(id)
	tk.StampCreated(time.Now(), tickets.Address{Project: tickets.ProjectName(projectDir), Epic: OneOffsEpic, ID: id}.String())
	out, err := schema.MarshalTicket(tk, "\n"+prompt+"\n")
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(epicDir, "issues", fmt.Sprintf("%s-%s.md", id, slug))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", "", err
	}
	if _, err := schema.ParseTicket(path); err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	return path, id, nil
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
