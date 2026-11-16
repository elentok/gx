package triageplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets"
)

// StartDesignPromptPrefix, at the start of a submitted prompt, makes the TUI
// fake play /gx-start-design.
const StartDesignPromptPrefix = "/gx-start-design "

// IsStartDesignPrompt reports whether text launches a Start Design chat.
func IsStartDesignPrompt(text string) bool { return strings.HasPrefix(text, StartDesignPromptPrefix) }

// The phrases a typed message to the Start Design chat may use; anything else
// is an edit that changes nothing and gets the card reprinted once.
const (
	// PhraseDesignYes starts the Design.
	PhraseDesignYes = "yes"
	// PhraseDesignDrop drops the n-th ticket of the card, counting from 1.
	PhraseDesignDrop = "drop <n>"
)

// maxDesignTickets is the most tickets a card proposes.
const maxDesignTickets = 14

// DesignCard is the "Start this Design?" card the fake proposes.
type DesignCard struct {
	Project, Backlog, Map, Title string
	Tickets                      []DesignTicket
}

// DesignTicket is one proposed ticket of a card.
type DesignTicket struct {
	ID, Address, Title string
}

// DesignPlayer plays the Start Design chat with the real gx verbs, as the skill
// would. The caller keeps the card between turns.
type DesignPlayer struct {
	Gx  string
	Env []string
	Dir string
}

// Text prints the card in the yes-no format the other fakes use.
func (c *DesignCard) Text() string {
	lines := []string{fmt.Sprintf("Start this Design?\n\nTitle: %s\nMap: %s\n", c.Title, c.Map)}
	for i, t := range c.Tickets {
		lines = append(lines, fmt.Sprintf("%d. %s: %s (same theme as the other %s tickets)", i+1, t.Address, t.Title, c.Backlog))
	}
	return strings.Join(lines, "\n") +
		"\n\n```gx-question\ntype: yes-no\nrecommended: yes\nyes: Creates the map and moves these tickets into it.\nno: Leaves the backlog as it is.\n```"
}

func (p *DesignPlayer) gx(args ...string) (string, error) {
	cmd := exec.Command(p.Gx, args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), p.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("%s", text)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Propose reads the backlog the prompt names and returns the card and its text.
func (p *DesignPlayer) Propose(prompt string) (*DesignCard, string, error) {
	fields := strings.Fields(prompt)
	if len(fields) < 2 {
		return nil, "", fmt.Errorf("the prompt names no backlog")
	}
	project, backlog, ok := strings.Cut(fields[1], ":")
	if !ok {
		return nil, "", fmt.Errorf("%q is not a project:backlog address", fields[1])
	}
	root, err := p.gx("tickets", "root")
	if err != nil {
		return nil, "", err
	}
	epic := tickets.LoadEpic(root, backlog)
	card := &DesignCard{Project: project, Backlog: backlog, Title: "Tidy the " + backlog + " backlog"}
	for _, t := range epic.Tickets {
		if st := epic.RenderedStatus(t); st != tickets.StatusOpen && st != tickets.StatusDraft {
			continue
		}
		if len(card.Tickets) < maxDesignTickets {
			card.Tickets = append(card.Tickets, DesignTicket{t.DisplayNumber(), project + ":" + backlog + "/" + t.DisplayNumber(), t.Title})
		}
	}
	if len(card.Tickets) == 0 {
		return nil, "", fmt.Errorf("%s has no open ticket", backlog)
	}
	// next-name prints project:name; the create verb takes the bare name.
	next, err := p.gx("tickets", "epic", "next-name", project+":"+filepath.Base(backlog)+"-cleanup")
	if err != nil {
		return nil, "", err
	}
	card.Map = strings.TrimPrefix(next, project+":")
	if _, err := p.gx("agent", "actionable", "Start this Design?"); err != nil {
		return nil, "", err
	}
	return card, "I chose one theme from the open tickets of " + backlog + ".\n\n" + card.Text(), nil
}

// React answers a typed message: yes creates the Design; anything else is an
// edit and reprints the card once.
func (p *DesignPlayer) React(card *DesignCard, typed string) (string, error) {
	typed = strings.TrimSpace(typed)
	if strings.EqualFold(typed, PhraseDesignYes) {
		return p.create(card)
	}
	if rest, ok := strings.CutPrefix(strings.ToLower(typed), "drop "); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && n >= 1 && n <= len(card.Tickets) && len(card.Tickets) > 1 {
			card.Tickets = append(card.Tickets[:n-1], card.Tickets[n:]...)
		}
	}
	return "Edit applied.\n\n" + card.Text(), nil
}

func (p *DesignPlayer) create(card *DesignCard) (string, error) {
	ids := make([]string, len(card.Tickets))
	for i, t := range card.Tickets {
		ids[i] = t.ID
	}
	address, err := p.gx("tickets", "start-design", card.Project+":"+card.Backlog, "--map", card.Map, "--title", card.Title, "--ticket", strings.Join(ids, ","))
	if err != nil {
		return "", err
	}
	out, err := p.gx("server", "web-url", "--json")
	if err != nil {
		return "Created " + address + ".", nil
	}
	var info struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(out), &info) != nil || info.URL == "" {
		return "Created " + address + ".", nil
	}
	return "Created the Design: " + strings.TrimRight(info.URL, "/") + "/#/d/" + address, nil
}
