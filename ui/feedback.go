package ui

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
)

// WorktreeLabel renders the current worktree's short name (the basename of its
// root path) for display in a panel frame title. When nerd fonts are enabled it
// is prefixed with the worktree icon; otherwise the bare name is returned.
func WorktreeLabel(worktreeRoot string, useNerdFont bool) string {
	name := filepath.Base(worktreeRoot)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	if useNerdFont {
		return Icons(true).Worktree + " " + name
	}
	return name
}

func JoinStatus(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "  ·  ")
}

func StatusWithHints(message string, hints ...key.Binding) string {
	hintText := RenderInlineBindings(hints...)
	if hintText == "" {
		return message
	}
	return JoinStatus(message, hintText)
}

func MessageComplete(action string) string {
	return strings.TrimSpace(action) + " complete"
}

func MessageAborted(action string) string {
	return strings.TrimSpace(action) + " aborted"
}

func MessageNoOutput() string {
	return "no command output"
}

func HintDismiss() string {
	return RenderInlineBindings(key.NewBinding(key.WithHelp("esc/enter/q", "dismiss")))
}

func HintDismissAndScroll() string {
	return JoinStatus(HintDismiss(), RenderInlineBindings(key.NewBinding(key.WithHelp("j/k", "scroll"))))
}

// HintFilter is the footer hint shown when a filterable view has no active
// filter: press '/' to start narrowing the list.
func HintFilter() string {
	return RenderInlineBindings(key.NewBinding(key.WithHelp("/", "filter")))
}

// HintClearFilter is the footer hint shown while a filter is active.
func HintClearFilter() string {
	return RenderInlineBindings(key.NewBinding(key.WithHelp("esc", "clear filter")))
}

func HintSubmitCancel() string {
	return RenderInlineBindings(
		key.NewBinding(key.WithHelp("enter", "submit")),
		key.NewBinding(key.WithHelp("esc", "cancel")),
	)
}

func HintChecklistConfirm() string {
	return RenderInlineBindings(
		key.NewBinding(key.WithHelp("space", "toggle")),
		key.NewBinding(key.WithHelp("a", "all")),
		key.NewBinding(key.WithHelp("enter", "confirm")),
		key.NewBinding(key.WithHelp("esc", "cancel")),
	)
}

func HintCancelScroll() string {
	return JoinStatus(
		RenderInlineBindings(key.NewBinding(key.WithHelp("ctrl+c", "cancel"))),
		RenderInlineBindings(key.NewBinding(key.WithHelp("j/k", "scroll"))),
	)
}
