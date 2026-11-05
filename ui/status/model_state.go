package status

import (
	"time"

	"github.com/elentok/gx/git"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/gitops"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/imagediff"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/status/stagemodal"
	"github.com/elentok/gx/ui/tree"
)

type Model struct {
	worktreeRoot string
	isBareRepo   bool
	settings     ui.Settings
	initialPath  string

	width  int
	height int
	ready  bool

	focus            focusPane
	diffarea         diffarea.Model
	diffContextLines int
	filetree         filetreePane

	err  error
	help help.Model

	diffLoader  diffLoader
	diffActions diffActions

	git gitops.Model

	modals stagemodal.Model
	keys   keys.Manager

	overlay             imagediff.Overlay
	fetchImageDiffBlobs func(file stage.StageFileStatus, cached bool) (old, new []byte, oldOK, newOK bool)
}

func DefaultSettings() ui.Settings {
	return ui.Settings{UseNerdFontIcons: true, ImageDiffs: true, Terminal: ui.TerminalPlain, DiffContextLines: 1, RenderHeartbeat: true}
}

type flashTickMsg struct{}
type renderTickMsg struct{}
type diffReloadMsg struct{ seq int }
type statusStartupLoadMsg struct{}

type branchSyncLoadedMsg struct {
	branchName string
	sync       git.SyncStatus
}

type commitFinishedMsg struct {
	err      error
	splitApp string // "tmux", "kitty", or "" for foreground
}

type lazygitLogFinishedMsg struct{ err error }
type editFileFinishedMsg struct {
	err      error
	splitApp string
}
type editCommentFinishedMsg struct {
	err      error
	splitApp string
}

const statusDiffReloadDebounce = 100 * time.Millisecond

func NewModel(worktreeRoot string, isBareRepo bool, settings ui.Settings, initialPath string, extraKeys keys.Manager) Model {
	if settings.DiffContextLines < 0 {
		settings.DiffContextLines = 0
	}
	if settings.DiffContextLines > 20 {
		settings.DiffContextLines = 20
	}
	statusKeys := newStatusManager()
	diffarreaModel := diffarea.NewModel(settings.UseNerdFontIcons)
	fileTreeModel := tree.NewModel[stage.StageFileStatus](toggleStageBinding, discardBinding)
	m := Model{
		worktreeRoot:     worktreeRoot,
		isBareRepo:       isBareRepo,
		settings:         settings,
		initialPath:      initialPath,
		diffContextLines: settings.DiffContextLines,
		help:             help.NewModel(append(help.BuildSections(statusKeys, *diffarreaModel.Keys(), *fileTreeModel.Keys(), extraKeys), diffHeaderHelpSection()), settings.About),
		keys:             statusKeys,
		focus:            focusFiletree,
		diffarea:         diffarreaModel,
		filetree:         filetreePane{model: fileTreeModel},
		git:              gitops.New(),

		overlay: imagediff.NewOverlay(imagediff.WriteToStdout, imagediff.DefaultDetectCapability),
		fetchImageDiffBlobs: func(file stage.StageFileStatus, cached bool) (old, new []byte, oldOK, newOK bool) {
			return stage.ImageDiffBlobs(worktreeRoot, file, cached)
		},
	}

	if settings.EnableNavigation {
		m.reloadFileList("")
	} else {
		m.reload("")
	}
	return m
}

func (m Model) KeyManager() keys.Manager {
	return m.keys
}
