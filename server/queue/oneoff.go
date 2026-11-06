package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
	"gopkg.in/yaml.v3"
)

// oneOffNameWords is how many of the prompt's first words seed a default name.
const oneOffNameWords = 4

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

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
func applyOneOffFile(req *wire.OneOffRequest) error {
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

func oneOffRefusal(reason, msg string) wire.OneOffResult {
	return wire.OneOffResult{Refused: true, Reason: reason, Message: msg}
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

// OneOffsDeps is what the one-offs need from the rest of the server.
type OneOffsDeps struct {
	Queue       *Queue
	Index       *index.Index
	Projects    *projects.Registry
	Registry    *runs.Registry
	Events      *broker.Broker
	Log         *slog.Logger
	Chat        *notify.ServerChat
	Host        Host
	TicketStore string
}

// OneOffs submits, announces and archives one-off tickets.
type OneOffs struct {
	OneOffsDeps
	sweepMu sync.Mutex               // one archive sweep at a time; the move's own writes wake the watcher
	backoff map[string]*sweepBackoff // by ticket address, under sweepMu
}

// NewOneOffs wires the one-offs to the parts of the server they use.
func NewOneOffs(d OneOffsDeps) *OneOffs { return &OneOffs{OneOffsDeps: d} }

// Submit creates a top-level ticket carrying the prompt and appends it to the
// queue in one call. Everything that can refuse is checked before the first
// write, and a refusal after the write removes the ticket again, so a refused
// submit leaves nothing behind for a retry to collide with.
func (o *OneOffs) Submit(req wire.OneOffRequest) (wire.OneOffResult, error) {
	var plan oneOffPlan
	if res, err := o.oneOffValidate(req, &plan); err != nil || res.Refused {
		return res, err
	}
	written, err := o.oneOffWrite(plan)
	if err != nil {
		return wire.OneOffResult{}, err
	}
	return o.oneOffEnqueue(plan, written)
}

// oneOffValidate checks the submit and fills plan. A refusal (or error) means
// plan is unusable; nothing has been written.
func (o *OneOffs) oneOffValidate(req wire.OneOffRequest, plan *oneOffPlan) (wire.OneOffResult, error) {
	if req.File != "" {
		if err := applyOneOffFile(&req); err != nil {
			var removed removedKeyError
			if errors.As(err, &removed) {
				return oneOffRefusal(wire.ReasonOptionRemoved, err.Error()), nil
			}
			return oneOffRefusal(wire.ReasonBadFile, err.Error()), nil
		}
		if req.Unique == "" && !req.NoUnique {
			resolved, err := filepath.EvalSymlinks(req.File)
			if err != nil {
				return oneOffRefusal(wire.ReasonBadFile, err.Error()), nil
			}
			if req.Unique, err = filepath.Abs(resolved); err != nil {
				return oneOffRefusal(wire.ReasonBadFile, err.Error()), nil
			}
		}
	}
	if req.NoUnique {
		req.Unique = ""
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return oneOffRefusal(wire.ReasonEmptyPrompt, "a one-off needs a prompt"), nil
	}
	typ := schema.TypePrompt
	switch schema.TicketType(req.Type) {
	case "", schema.TypePrompt:
	case schema.TypeEvolve, schema.TypeDiagnose:
		typ = schema.TicketType(req.Type)
	default:
		return oneOffRefusal(wire.ReasonBadType, "type "+req.Type+" is not supported by a one-off"), nil
	}
	project, dir, err := o.oneOffProject(req)
	if err != nil {
		return wire.OneOffResult{}, err
	}
	if dir == "" {
		return oneOffRefusal(wire.ReasonUnknownProj, "no project "+project+" in the ticket store"), nil
	}

	slug := req.Name
	if slug == "" {
		slug = defaultOneOffName(prompt)
	}
	if _, err := tickets.ParseAddress(tickets.Address{Project: project, Epic: slug, ID: "01"}.String(), tickets.AddressContext{}); err != nil {
		return oneOffRefusal(wire.ReasonInvalidAddress, "name "+slug+" is not usable in a ticket address"), nil
	}
	if dup, err := liveDuplicate(dir, req.Unique); err != nil {
		return wire.OneOffResult{}, err
	} else if dup != nil {
		return dup.refusal(project, dir)
	}
	tk := schema.Ticket{Status: schema.StatusOpen, Type: typ, Notify: req.Notify, Unique: req.Unique, ExpectedContextWindow: req.ExpectedContextWindow}
	if refused := o.oneOffOptionsRefusal(project, req); refused != nil {
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
	return wire.OneOffResult{}, nil
}

// oneOffWrite puts the ticket on disk in the shared epic.
func (o *OneOffs) oneOffWrite(p oneOffPlan) (writtenOneOff, error) {
	path, id, err := o.writeOneOffTicket(p.dir, p.slug, p.tk, p.prompt)
	if err != nil {
		return writtenOneOff{}, err
	}
	return writtenOneOff{addr: tickets.Address{Project: p.project, Epic: p.epic, ID: id}, path: path}, nil
}

// oneOffEnqueue queues the written ticket and only then records `submitted`.
// Any failure removes the new ticket file again so nothing unqueued stays
// behind; the shared epic stays.
func (o *OneOffs) oneOffEnqueue(p oneOffPlan, w writtenOneOff) (wire.OneOffResult, error) {
	res, err := o.oneOffQueue(p, w)
	if err != nil || res.Refused {
		if rmErr := os.Remove(w.path); rmErr != nil {
			o.Log.Warn("remove unqueued one-off", "ticket", w.addr.String(), "err", rmErr)
		}
		if refreshErr := o.Index.Refresh(); refreshErr != nil {
			o.Log.Warn("refresh after one-off cleanup", "err", refreshErr)
		}
		return res, err
	}
	if err := runlog.AppendEvent(p.dir, p.epic, runlog.Event{
		Time: time.Now(), Type: string(events.Submitted), Ticket: w.addr.ID, Address: w.addr.String(),
	}); err != nil {
		return wire.OneOffResult{}, err
	}
	return res, nil
}

func (o *OneOffs) oneOffQueue(p oneOffPlan, w writtenOneOff) (wire.OneOffResult, error) {
	// Queue.Add checks the index, so it must see the new ticket first.
	if err := o.Index.Refresh(); err != nil {
		return wire.OneOffResult{}, err
	}
	q, err := o.Queue.queueRoot(w.addr, p.agent, p.front)
	if err != nil {
		return wire.OneOffResult{}, err
	}
	if q.Refused {
		return oneOffRefusal(q.Reason, q.Message), nil
	}
	return wire.OneOffResult{Address: w.addr.String(), Status: wire.OneOffStatusQueued}, nil
}

// queueRoot queues the root of addr. The shared epic is queued once, as a root:
// a later submit only moves it.
func (q *Queue) queueRoot(addr tickets.Address, agent string, front bool) (wire.QueueResult, error) {
	root := RootOf(addr)
	if !q.RootQueued(root) {
		return q.Add(wire.QueueRequest{Address: addr.String(), Agent: agent, Front: front})
	}
	res, err := q.queueRaise(root, front)
	var agents []string
	for _, it := range q.List() {
		if a, perr := tickets.ParseAddress(it.Address, tickets.AddressContext{}); perr == nil && RootOf(a) == root {
			agents = append(agents, it.Agent)
		}
	}
	return q.WithExtraUsage(root.Project, agents, res, err)
}

// queueRaise moves every queued item of root to the head, keeping their order.
// Without front the queue is left as it is.
func (q *Queue) queueRaise(root RootRef, front bool) (wire.QueueResult, error) {
	return q.Write(root.String(), func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		if !front {
			return items, nil
		}
		var head, rest []wire.QueueItem
		for _, it := range items {
			if a, err := tickets.ParseAddress(it.Address, tickets.AddressContext{}); err == nil && RootOf(a) == root {
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
func liveDuplicate(projectDir, key string) (*duplicate, error) {
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
func (d *duplicate) refusal(project, projectDir string) (wire.OneOffResult, error) {
	addr := tickets.Address{Project: project, Epic: d.epic, ID: d.id}.String()
	if err := runlog.AppendEvent(projectDir, d.epic, runlog.Event{
		Time: time.Now(), Type: string(events.SubmitRefused), Ticket: d.id, Address: addr,
		Kind: string(events.DuplicateLive), Reason: "a live ticket already has this dedupe key",
	}); err != nil {
		return wire.OneOffResult{}, err
	}
	res := oneOffRefusal(wire.ReasonDuplicateLive, addr+" already has this dedupe key")
	res.Address = addr
	return res, nil
}

// NotifyResult sends a merged ticket's ## Result to chat when its submit asked
// for it. A ticket without the flag stays silent; pauses never reach here.
func (o *OneOffs) NotifyResult(addr tickets.Address, ticketPath string) {
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || !tk.Notify {
		return
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		o.Log.Warn("read result", "ticket", addr.String(), "err", err)
		return
	}
	dir := filepath.Dir(filepath.Dir(filepath.Dir(ticketPath))) // project/epic/issues/file
	o.Chat.Result(addr.Project, o.Projects.ChatOverride(addr.Project), dir, addr.Epic, addr.String(), schema.Section(schema.ParseBody(string(raw)), "Result"))
}

// oneOffOptionsRefusal checks the submit options before anything is written. A
// blocker must be a ticket of the same project (no cross-project blocking).
func (o *OneOffs) oneOffOptionsRefusal(project string, req wire.OneOffRequest) *wire.OneOffResult {
	refuse := func(reason, msg string) *wire.OneOffResult {
		r := oneOffRefusal(reason, msg)
		return &r
	}
	if req.ExpectedContextWindow < 0 {
		return refuse(wire.ReasonBadWindow, "expected context window must be non-negative")
	}
	if len(req.BlockedBy) == 0 {
		return nil
	}
	if err := o.Index.Refresh(); err != nil {
		r := oneOffRefusal(wire.ReasonBadBlocker, err.Error())
		return &r
	}
	for _, b := range req.BlockedBy {
		a, err := tickets.ParseAddress(b, tickets.AddressContext{})
		switch {
		case err != nil:
			return refuse(wire.ReasonBadBlocker, err.Error())
		case a.Project != project:
			return refuse(wire.ReasonBadBlocker, b+" is in another project (cross-project blocking not supported)")
		case !o.Queue.hasTicket(a.String()):
			return refuse(wire.ReasonBadBlocker, "no ticket "+b)
		}
	}
	return nil
}

// oneOffProject picks the project: the explicit one, else the registered
// project owning Cwd, else scratch. dir is empty when the project is unknown.
func (o *OneOffs) oneOffProject(req wire.OneOffRequest) (project, dir string, err error) {
	dirs, err := tickets.ProjectDirs(o.TicketStore)
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
	cwd := projects.CanonicalDir(req.Cwd)
	best, bestLen := projects.ScratchProject, -1
	for _, d := range dirs {
		name := tickets.ProjectName(d)
		pf, err := config.ReadProjectFile(d)
		if name == projects.ScratchProject || err != nil || pf.Repo == nil {
			continue
		}
		for _, root := range projectRoots(projects.CanonicalDir(*pf.Repo)) {
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

// OneOffFile is the name of a one-off's single ticket; nothing else writes it.
const OneOffFile = "01-one-off.md"

// The epics holding one-off tickets: every ticket in OneOffsEpic is a one-off,
// whoever wrote it. OneOffsArchivedEpic is where finished ones are moved.
const (
	OneOffsEpic         = tickets.OneOffsEpic
	OneOffsArchivedEpic = tickets.ArchivedOneOffsEpic
)

func IsOneOffsEpicName(name string) bool {
	return name == OneOffsEpic || name == OneOffsArchivedEpic
}

// IsOneOffTicket reports whether ticketPath (project/epic/issues/file) is a
// one-off's ticket: in a one-offs epic, or the single ticket of an old
// one-epic-per-one-off submit.
func IsOneOffTicket(ticketPath string) bool {
	epic := filepath.Base(filepath.Dir(filepath.Dir(ticketPath)))
	return IsOneOffsEpicName(epic) || filepath.Base(ticketPath) == OneOffFile
}

// IsOneOffEpic reports whether e is a one-off epic: one of the one-offs epics,
// or an old single-ticket epic whose ticket is the one writeOneOffTicket wrote.
func IsOneOffEpic(e tickets.Epic) bool {
	if IsOneOffsEpicName(filepath.Base(e.Path)) {
		return true
	}
	for _, t := range e.Tickets {
		if IsOneOffTicket(t.FilePath()) {
			return true
		}
	}
	return false
}

// nextOneOffNumber is above the highest in the live epic and the archived one.
func nextOneOffNumber(projectDir string, epic *tickets.Epic) (int, error) {
	next := 1
	if id, err := ticketmove.NextTicketID(*epic, ""); err != nil {
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
func (o *OneOffs) writeOneOffTicket(projectDir, slug string, tk schema.Ticket, prompt string) (path, id string, err error) {
	epicDir := filepath.Join(projectDir, OneOffsEpic)
	if err := o.Host.EnsureDraftEpic(epicDir); err != nil {
		return "", "", err
	}
	epic, unlock, err := ticketmove.LoadLockedEpic(epicDir)
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

func (o *OneOffs) handle(w http.ResponseWriter, r *http.Request) {
	var req wire.OneOffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := o.Submit(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}
