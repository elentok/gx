package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

// Refusal reasons of a one-off submit.
const (
	ReasonEmptyPrompt   = "empty-prompt"
	ReasonUnknownProj   = "unknown-project"
	ReasonBadTicketType = "invalid-type"
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
	// Type defaults to prompt.
	Type string `json:"type,omitempty"`
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

// oneOff creates a top-level ticket carrying the prompt and appends it to the
// queue in one call.
func (s *Server) oneOff(req OneOffRequest) (OneOffResult, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return oneOffRefusal(ReasonEmptyPrompt, "a one-off needs a prompt"), nil
	}
	typ := schema.TicketType(req.Type)
	if req.Type == "" {
		typ = schema.TypePrompt
	}
	if !typ.Valid() {
		return oneOffRefusal(ReasonBadTicketType, fmt.Sprintf("unknown ticket type %q", req.Type)), nil
	}
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return oneOffRefusal(ReasonSchedulerNotSelected, `orchestrator is not "server"`), nil
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
	if err := writeOneOffTicket(dir, epic, typ, prompt); err != nil {
		if os.IsExist(err) {
			return oneOffRefusal(ReasonNameTaken, project+":"+epic+" already exists"), nil
		}
		return OneOffResult{}, err
	}
	if err := ralphloop.AppendEvent(dir, epic, ralphloop.Event{
		Time: time.Now(), Type: string(events.Submitted), Ticket: addr.ID, Address: addr.String(),
	}); err != nil {
		return OneOffResult{}, err
	}
	// queueAdd checks the index, so it must see the new ticket first.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		return OneOffResult{}, err
	}
	q, err := s.queueAdd(QueueRequest{Address: addr.String(), Agent: req.Agent})
	if err != nil {
		return OneOffResult{}, err
	}
	if q.Refused {
		return oneOffRefusal(q.Reason, q.Message), nil
	}
	return OneOffResult{Address: addr.String(), Status: OneOffStatusQueued}, nil
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

// writeOneOffTicket creates the epic directory and its single ticket. The
// directory creation is the uniqueness check: an existing epic is os.ErrExist.
func writeOneOffTicket(projectDir, epic string, typ schema.TicketType, prompt string) error {
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
	out, err := schema.MarshalTicket(schema.Ticket{ID: "01", Status: schema.StatusOpen, Type: typ}, "\n"+prompt+"\n")
	if err != nil {
		return err
	}
	path := filepath.Join(issues, "01-one-off.md")
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
