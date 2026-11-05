// Package projects holds the project registry, the start-up migrator and the
// checks reads and saves, with their routes. It does not import server.
package projects

import (
	"cmp"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/tickets"
)

// RegistryBack is what the Registry reads from the Server.
type RegistryBack interface {
	SessionFinished(sess agentrunner.Session) bool
	// BusyAddresses is the address of every queued ticket, then of every run.
	BusyAddresses() []string
}

// Env is the part of the Server's Config that Registry, Migrator and Checks read.
type Env struct {
	TicketStore                  string
	StateDir                     string
	Env                          []string
	Migrations                   *migrations.Registry
	MigrationFailurePollInterval time.Duration
	Chat                         ralphloop.ServerChatConfig
	GuardPoll                    time.Duration
	InstallDeps                  func(path, logDir string) ([]string, error)
	RateLimitResetBuffer         time.Duration
	RunnerWait                   time.Duration
	Sleep                        func(time.Duration)
}

// The three types hold a getter for the Server's config and call it on every
// read, so a later change to the config shows.

// Registry is project lookup and edit: the store's project dirs, their
// project.json, the env-derived dirs and the typing rules.
type Registry struct {
	cfg         func() Env
	log         *slog.Logger
	idx         *index.Index
	settings    *settings.Holder
	chat        *ralphloop.ServerChat
	back        RegistryBack
	unavailable unavailableNotes
}

// NewRegistry builds the Registry.
func NewRegistry(cfg func() Env, log *slog.Logger, idx *index.Index, set *settings.Holder, chat *ralphloop.ServerChat, back RegistryBack) *Registry {
	return &Registry{cfg: cfg, log: log, idx: idx, settings: set, chat: chat, back: back}
}

// NewMigrator builds the Migrator.
func NewMigrator(cfg func() Env, log *slog.Logger, back MigratorBack) *Migrator {
	return &Migrator{cfg: cfg, log: log, back: back, done: make(chan struct{}), startupDone: make(chan struct{})}
}

// NewChecks builds Checks over the Registry.
func NewChecks(cfg func() Env, reg *Registry, guards *runs.Guards) *Checks {
	return &Checks{cfg: cfg, projects: reg, guards: guards}
}

// Dir finds the project's directory in the store.
func (p *Registry) Dir(project string) (string, error) {
	dirs, err := tickets.ProjectDirs(p.cfg().TicketStore)
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
	dirs, err := tickets.ProjectDirs(p.cfg().TicketStore)
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

// MergingOnMerged reports whether the project uses the on-merged merging policy.
// An unreadable project file falls back to the default policy.
func (p *Registry) MergingOnMerged(project string) bool {
	dir, err := p.Dir(project)
	if err != nil {
		return true
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.LandingPolicy() == config.LandingOnLanded
}

// WorktreeDir is where the project's repo keeps its linked worktrees.
func (p *Registry) WorktreeDir(project string) string {
	pf, err := config.ReadProjectFile(filepath.Join(p.cfg().TicketStore, project))
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
	if p.cfg().Sleep != nil {
		deps.Sleep = p.cfg().Sleep
	}
	if p.cfg().InstallDeps != nil {
		deps.InstallDeps = p.cfg().InstallDeps
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
		GateStatePath:    p.cfg().Chat.GateStatePath,
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

// LaunchHarness is the harness a new interactive session in project starts on
// when its request names none. Unlike the Default harness it keeps an
// unregistered name, so createSession refuses it instead of silently running
// Claude.
func (p *Registry) LaunchHarness(project string) string {
	return cmp.Or(p.AgentsConfig(project).DefaultHarness, string(ralphloop.AgentClaude))
}

// RunAgentConfig is the model and effort run's agent launched under, else
// the project's agents.* config. Every launch, finish and adoption resolves
// them here.
func (p *Registry) RunAgentConfig(project string, agent ralphloop.AgentKind, run runs.Run) (model, effort string) {
	model, effort = ralphloop.ResolveTier(p.AgentsConfig(project), agent, ralphloop.TierWorker)
	return cmp.Or(run.Model, model), cmp.Or(run.Effort, effort)
}
