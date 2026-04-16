package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

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

func MessageOpening(target string) string {
	return "opening " + strings.TrimSpace(target) + "..."
}

func MessageClosed(target string) string {
	return strings.TrimSpace(target) + " closed"
}

func HintDismiss() string {
	return RenderInlineBindings(key.NewBinding(key.WithHelp("esc/enter/q", "dismiss")))
}

func HintDismissScroll() string {
	return JoinStatus(HintDismiss(), RenderInlineBindings(key.NewBinding(key.WithHelp("j/k", "scroll"))))
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
