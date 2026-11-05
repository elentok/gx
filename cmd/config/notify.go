package config

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gxconfig "github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/tickets"
)

// NewNotify returns `gx notify`.
func NewNotify(d Deps) *cobra.Command {
	var enable, disable, project string
	var status, testBatch bool

	cmd := &cobra.Command{
		Use:   "notify [message]",
		Short: "send a message via configured Telegram/Slack notifications",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runNotify(args, enable, disable, project, status, testBatch, d)
		},
	}
	cmd.Flags().StringVar(&enable, "enable", "", "clear the manual mute for every destination of a transport (telegram/slack)")
	cmd.Flags().StringVar(&disable, "disable", "", "trip the manual mute for every destination of a transport (telegram/slack)")
	cmd.Flags().StringVar(&project, "project", "", "with --enable/--disable: only that project's destination")
	cmd.Flags().BoolVar(&status, "status", false, "report per-destination mute state, its projects and muted tickets")
	cmd.Flags().BoolVar(&testBatch, "test-batch", false, "send a fixed 2-message batch through the same join a real flush uses, to reproduce/verify the batch-separator escaping bug live")
	return cmd
}

func isKnownNotifyTransport(name string) bool {
	return slices.Contains(notify.NotifyTransports, name)
}

// notifyDestinations lists every notification destination: the global one per
// transport plus any a project's override adds.
func notifyDestinations(d Deps) ([]notify.Destination, error) {
	load := d.LoadConfig
	if load == nil {
		load = d.Dirs().Load
	}
	cfg, err := load()
	if err != nil {
		return nil, err
	}
	dirs, err := tickets.ProjectDirs(cfg.TicketStore.Path)
	if err != nil {
		return nil, err
	}
	overrides := map[string]*gxconfig.NotificationsConfig{}
	for _, dir := range dirs {
		var n *gxconfig.NotificationsConfig
		if pf, err := gxconfig.ReadProjectFile(dir); err == nil {
			n = pf.Notifications
		}
		overrides[tickets.ProjectName(dir)] = n
	}
	return notify.NotificationDestinations(cfg.Notifications, overrides), nil
}

// runNotify dispatches `gx notify` to whichever of its mutually exclusive
// modes was requested: send a message, flip a transport's manual mute, or
// report status. Args holds at most one positional message (Args:
// cobra.MaximumNArgs(1)).
func runNotify(args []string, enable, disable, project string, status, testBatch bool, d Deps) error {
	hasMessage := len(args) == 1
	controlFlags := 0
	if enable != "" {
		controlFlags++
	}
	if disable != "" {
		controlFlags++
	}
	if status {
		controlFlags++
	}
	if testBatch {
		controlFlags++
	}

	switch {
	case hasMessage && controlFlags > 0:
		return fmt.Errorf("a message and --enable/--disable/--status/--test-batch are mutually exclusive")
	case controlFlags > 1:
		return fmt.Errorf("--enable, --disable, --status, and --test-batch are mutually exclusive")
	case project != "" && enable == "" && disable == "":
		return fmt.Errorf("--project only narrows --enable/--disable")
	case enable != "":
		return runNotifySetMute(enable, project, false, d)
	case disable != "":
		return runNotifySetMute(disable, project, true, d)
	case status:
		return runNotifyStatus(d)
	case testBatch:
		return sendTestBatch(d)
	case hasMessage:
		return sendNotify(args[0], d)
	default:
		return fmt.Errorf("notify requires a message or one of --enable/--disable/--status/--test-batch")
	}
}

// sendNotify sends text to every notification service configured in
// ~/.config/gx/config.json (Telegram and/or Slack), no-op'ing per-service
// when unconfigured, and reports which service(s) it sent to.
func sendNotify(text string, d Deps) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}

	statePath, err := notificationStatePath(d.Dirs())
	if err != nil {
		return err
	}
	sent, err := notify.SendMessage(cfg.Notifications, statePath, text)
	if len(sent) == 0 {
		if err == nil {
			fmt.Fprintf(d.Stdout, "no notification service configured (see `gx config edit`)\n")
		}
		return err
	}
	fmt.Fprintf(d.Stdout, "sent to: %s\n", strings.Join(sent, ", "))
	return err
}

// sendTestBatch sends a fixed 2-message batch to every notification service
// configured in ~/.config/gx/config.json, through the exact same renderBatch
// join a real flush uses — reproduces (pre-fix) or verifies (post-fix) the
// batch-separator MarkdownV2 escaping bug against the real APIs, rather than
// only in a unit test.
func sendTestBatch(d Deps) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}

	sent, err := notify.SendTestBatch(cfg.Notifications)
	if len(sent) == 0 {
		if err == nil {
			fmt.Fprintf(d.Stdout, "no notification service configured (see `gx config edit`)\n")
		}
		return err
	}
	fmt.Fprintf(d.Stdout, "sent test batch to: %s\n", strings.Join(sent, ", "))
	return err
}

// runNotifySetMute flips the manual mute of every destination of transport
// (only project's, when set) in the notification state file and reports the
// new state. This is the same manual path a planned quiet period would use.
func runNotifySetMute(transport, project string, muted bool, d Deps) error {
	if !isKnownNotifyTransport(transport) {
		return fmt.Errorf("unknown transport %q (expected telegram or slack)", transport)
	}
	all, err := notifyDestinations(d)
	if err != nil {
		return err
	}
	var targets []notify.Destination
	for _, dest := range all {
		if dest.Transport == transport && (project == "" || slices.Contains(dest.Projects, project)) {
			targets = append(targets, dest)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("project %q has no %s destination", project, transport)
	}

	statePath, err := notificationStatePath(d.Dirs())
	if err != nil {
		return err
	}
	err = notify.UpdateNotificationState(statePath, func(state *notify.NotificationState) {
		for _, dest := range targets {
			ts := state.Transports[dest.Key]
			ts.Muted = muted
			if muted {
				ts.Reason = "manual-disable"
				ts.TrippedAt = time.Now()
			} else {
				ts.Reason = ""
				ts.TrippedAt = time.Time{}
			}
			state.Transports[dest.Key] = ts
		}
	})
	if err != nil {
		return err
	}

	for _, dest := range targets {
		if muted {
			fmt.Fprintf(d.Stdout, "%s: muted\n", dest.Key)
		} else {
			fmt.Fprintf(d.Stdout, "%s: active\n", dest.Key)
		}
	}
	return nil
}

// notificationStatePath is the real notification state file, under gx's state
// dir.
func notificationStatePath(dirs gxconfig.Dirs) (string, error) {
	dir, err := dirs.StateDir()
	if err != nil {
		return "", err
	}
	return notify.NotificationStatePath(dir), nil
}

// runNotifyStatus reports every destination's mute state and projects, plus
// every ticket under the current repo's tracker root carrying a non-empty Mutes.
func runNotifyStatus(d Deps) error {
	statePath, err := notificationStatePath(d.Dirs())
	if err != nil {
		return err
	}
	state, err := notify.LoadNotificationState(statePath)
	if err != nil {
		return err
	}
	dests, err := notifyDestinations(d)
	if err != nil {
		return err
	}
	for _, dest := range dests {
		line := fmt.Sprintf("%s: active", dest.Key)
		if ts := state.Transports[dest.Key]; ts.Muted {
			line = fmt.Sprintf("%s: muted (%s)", dest.Key, ts.Reason)
		}
		if len(dest.Projects) > 0 {
			line += " [projects: " + strings.Join(dest.Projects, ", ") + "]"
		}
		fmt.Fprintln(d.Stdout, line)
	}

	cwd, err := d.Getwd()
	if err != nil {
		return err
	}
	root, err := tickets.RootForDirs(d.Dirs(), cwd)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}

	var muted []string
	for _, epic := range epics {
		for _, t := range epic.Tickets {
			for _, m := range t.Mutes {
				muted = append(muted, fmt.Sprintf("%s/%s: %s (tripped %s)",
					epic.Name, t.DisplayNumber(), m.EventType, m.TrippedAt.Format(time.RFC3339)))
			}
		}
	}

	if len(muted) == 0 {
		fmt.Fprintf(d.Stdout, "\nno tickets with active mutes\n")
		return nil
	}
	fmt.Fprintf(d.Stdout, "\nmuted tickets:\n")
	for _, line := range muted {
		fmt.Fprintf(d.Stdout, "  %s\n", line)
	}
	return nil
}
