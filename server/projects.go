package server

import (
	"cmp"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/tickets"
)

// registryBack is what the Registry reads from the Server.
type registryBack interface {
	sessionFinished(sess agentrunner.Session) bool
	launchHarness(project string) string
	// busyAddresses is the address of every queued ticket, then of every run.
	busyAddresses() []string
}

// Registry is project lookup and edit: the store's project dirs, their
// project.json, the env-derived dirs and the typing rules.
type Registry struct {
	cfg         *Config // the Server's own, so a later change to it shows here
	log         *slog.Logger
	idx         *index.Index
	settings    *settings.Holder
	chat        *ralphloop.ServerChat
	back        registryBack
	unavailable unavailableNotes
}

// initProjects builds the projects Registry, Migrator and Checks from the
// Server's own fields. Tests that build a bare Server call it too.
func (s *Server) initProjects() {
	s.projects = &Registry{cfg: &s.cfg, log: s.log, idx: s.idx, settings: &s.settings, chat: s.chat, back: s}
	s.migrator = &Migrator{cfg: &s.cfg, log: s.log, back: s, done: make(chan struct{}), startupDone: make(chan struct{})}
	s.checks = &Checks{cfg: &s.cfg, projects: s.projects, guards: s.guards}
}

// withProjects is initProjects for a bare Server literal in a test.
func withProjects(s *Server) *Server {
	s.initProjects()
	return s
}

// busyAddresses implements registryBack.
func (s *Server) busyAddresses() []string {
	var out []string
	for _, it := range s.queued.list() {
		out = append(out, it.Address)
	}
	for _, r := range s.registry.List() {
		out = append(out, r.Address)
	}
	return out
}

// Dir finds the project's directory in the store.
func (p *Registry) Dir(project string) (string, error) {
	dirs, err := tickets.ProjectDirs(p.cfg.TicketStore)
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) == project {
			return d, nil
		}
	}
	return "", fmt.Errorf("no project %s in the ticket store", project)
}

// Of finds the project's directory in the store and the repo its agents run in.
func (p *Registry) Of(project string) (dir, repo string, err error) {
	d, err := p.Dir(project)
	if err != nil {
		return "", "", err
	}
	pf, err := config.ReadProjectFile(d)
	if err != nil {
		return "", "", err
	}
	if pf.Repo == nil {
		return "", "", fmt.Errorf("project %s has no repo", project)
	}
	return d, *pf.Repo, nil
}

// Epics is the named project's epics as of the last store scan; a project the
// scan has not seen yet is read from disk.
func (p *Registry) Epics(project string) ([]tickets.Epic, error) {
	dirs, err := tickets.ProjectDirs(p.cfg.TicketStore)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if tickets.ProjectName(dir) != project {
			continue
		}
		if epics, ok := p.idx.EpicsOf(dir); ok {
			return epics, nil
		}
		return tickets.LoadResolved(project, dir)
	}
	return nil, &tickets.AddressError{Code: tickets.CodeUnknownProject, Msg: "no project " + project}
}

// WorktreeDir is where the project's repo keeps its linked worktrees.
func (p *Registry) WorktreeDir(project string) string {
	pf, err := config.ReadProjectFile(filepath.Join(p.cfg.TicketStore, project))
	if err != nil || pf.Repo == nil {
		return ""
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return ""
	}
	return repo.LinkedWorktreeDir()
}

// DepsWith is the default deps talking to runner.
func (p *Registry) DepsWith(runner agentrunner.Runner) ralphloop.Deps {
	deps := p.DefaultDeps()
	deps.Runner = runner
	if p.cfg.Sleep != nil {
		deps.Sleep = p.cfg.Sleep
	}
	if p.cfg.InstallDeps != nil {
		deps.InstallDeps = p.cfg.InstallDeps
	}
	return deps
}

// ChatOverride is the project's own notification block from project.json, nil
// when it has none (or is unreadable) so its notifications use the global one.
func (p *Registry) ChatOverride(project string) *ralphloop.ServerChatConfig {
	dir, err := p.Dir(project)
	if err != nil {
		return nil
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Notifications == nil {
		return nil
	}
	n := pf.Notifications
	return &ralphloop.ServerChatConfig{
		TelegramBotToken: n.Telegram.BotToken,
		TelegramChatID:   n.Telegram.ChatID,
		SlackWebhookURL:  n.Slack.WebhookURL,
		GateStatePath:    p.cfg.Chat.GateStatePath,
	}
}

// AgentsConfig is config agents.* for project, with the project's overrides on
// top.
func (p *Registry) AgentsConfig(project string) config.AgentsConfig {
	set := p.settings.Load()
	cfg := config.Config{Agents: config.AgentsConfig{Claude: set.AgentsClaude, Codex: set.AgentsCodex, DefaultHarness: set.AgentsDefaultHarness}}
	if dir, err := p.Dir(project); err == nil {
		if pf, err := config.ReadProjectFile(dir); err == nil {
			cfg = cfg.WithProject(pf)
		}
	}
	return cfg.Agents
}

// RunAgentConfig is the model and effort run's agent launched under, else
// the project's agents.* config. Every launch, finish and adoption resolves
// them here.
func (p *Registry) RunAgentConfig(project string, agent ralphloop.AgentKind, run runs.Run) (model, effort string) {
	model, effort = ralphloop.ResolveTier(p.AgentsConfig(project), agent, ralphloop.TierWorker)
	return cmp.Or(run.Model, model), cmp.Or(run.Effort, effort)
}
