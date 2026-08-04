package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/cli/confirm"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/app"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func runWorktrees(_ string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	repo, err := git.FindRepo(cwd)
	if err != nil {
		return err
	}

	if problem := git.CheckFetchConfig(repo.Root); problem != nil {
		cmdList := strings.Join(problem.Commands, "\n  ")
		prompt := fmt.Sprintf(
			"%s\n\nWorktree statuses may not show correctly without this.\n\nFix by running:\n  %s",
			problem.Description, cmdList,
		)
		confirmed, err := confirm.Run(prompt)
		if err != nil {
			return err
		}
		if confirmed {
			if err := git.FixFetchConfig(repo.Root); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to fix fetch config: %v\n", err)
			}
		}
	}

	// Detect which worktree the user launched from, if any.
	var activeWorktreePath string
	if info, err := git.IdentifyDir(cwd); err == nil {
		activeWorktreePath = info.WorktreeRoot
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: activeWorktreePath,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runStatus(target string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx status must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	initialPath := ""
	if strings.TrimSpace(target) != "" {
		initialPath, err = resolveStatusTargetPath(root, cwd, target)
		if err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: root, InitialPath: initialPath},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runLog(opts LogOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx log must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: root, Ref: opts.Ref, FilterPath: opts.File},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runShow(ref string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx show must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: root, Ref: ref},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runStash() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx stash must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStash, WorktreeRoot: root},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runPRs(allRepos bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx prs must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabPRs, WorktreeRoot: root, AllRepos: allRepos},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func runTickets(allRepos bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return err
	}
	if info.Repo.IsBare && info.WorktreeRoot == "" {
		return fmt.Errorf("gx tickets must be run from a regular repo or linked worktree")
	}

	root, err := git.WorktreeRoot(cwd)
	if err != nil {
		return err
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	m := app.New(*repo, app.Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: root, AllRepos: allRepos},
		ActiveWorktreePath: root,
		Settings:           settingsFromConfig(cfg),
	})
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

func settingsFromConfig(cfg config.Config) ui.Settings {
	return ui.Settings{
		UseNerdFontIcons: cfg.UseNerdFontIcons,
		ImageDiffs:       cfg.ImageDiffs,
		InputModalBottom: cfg.InputModalBottom,
		Terminal:         ui.DetectTerminal(),
		EnableNavigation: true,
		DiffContextLines: cfg.StageDiffContextLines,
		NameAliases:      cfg.NameAliases,
		LogConfig:        cfg.Log,
		ExecutionQueue:   cfg.ExecutionQueue,
		Notifications:    cfg.Notifications,
	}
}

func resolveStatusTargetPath(worktreeRoot, cwd, target string) (string, error) {
	path := filepath.Clean(target)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	root := filepath.Clean(worktreeRoot)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", fmt.Errorf("status target must be a file inside %s", worktreeRoot)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("status target %q is outside worktree root %s", target, worktreeRoot)
	}
	return filepath.ToSlash(rel), nil
}
