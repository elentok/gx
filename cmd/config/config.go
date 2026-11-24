// Package config is `gx config` (show, edit, defaults, test-notifications) and
// `gx notify`.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	gxconfig "github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/wire"
)

// Deps is the runtime context of `gx config` and `gx notify`.
type Deps struct {
	cmdbase.Env
	// InitConfig writes the default config file and returns its path.
	InitConfig func() (string, error)
	// RunEditor opens path in editor.
	RunEditor func(editor, path string, in io.Reader, out, err io.Writer) error
}

// DefaultDeps returns the production hooks; the caller sets Env.
func DefaultDeps(dirs gxconfig.Dirs, env []string) Deps {
	return Deps{
		InitConfig: dirs.Init,
		RunEditor: func(editor, path string, in io.Reader, out, errOut io.Writer) error {
			return cmdbase.RunEditorCommand(env, editor, path, in, out, errOut)
		},
	}
}

// New returns `gx config`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "manage gx configuration",
	}
	show := &cobra.Command{
		Use:   "show",
		Short: "print the effective (merged) config as JSON, secrets redacted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			return runConfigShow(d, reveal)
		},
	}
	get := &cobra.Command{
		Use:   "get <key>",
		Short: "print one effective config value (dotted path, e.g. skills.code-review)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			return runConfigGet(d, args[0], reveal)
		},
	}
	for _, c := range []*cobra.Command{show, get} {
		c.Flags().Bool("reveal", false, "print secret values (bot tokens, chat ids, webhooks) instead of redacting them")
	}
	cmd.AddCommand(
		show,
		get,
		&cobra.Command{
			Use:   "edit",
			Short: "open the config file in $EDITOR (creates it if missing)",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runEditConfig(d)
			},
		},
		&cobra.Command{
			Use:   "defaults",
			Short: "print the built-in default config as JSON",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigDefaults(d)
			},
		},
		testNotifications(d),
	)
	return cmd
}

func testNotifications(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "test-notifications",
		Short: "send a test message on every notification channel",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runConfigTestNotifications(c, d, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit each channel's outcome as JSON")
	return cmd
}

func runEditConfig(d Deps) error {
	path, err := d.Dirs().FilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		createdPath, initErr := d.InitConfig()
		if initErr != nil {
			return initErr
		}
		fmt.Fprintf(d.Stdout, "Created config file at %s\n", createdPath)
	} else if err != nil {
		return err
	}

	editor := d.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return fmt.Errorf("$EDITOR is not set")
	}
	return d.RunEditor(editor, path, d.Stdin, d.Stdout, d.Stderr)
}

func runConfigShow(d Deps, reveal bool) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}
	tree, _, err := toTree(cfg, reveal)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%s\n", b)
	return nil
}

// runConfigGet prints one value: a string bare, anything else as JSON. A key
// that is, or contains, a secret is refused without --reveal rather than
// printed redacted, so a script never mistakes the placeholder for the value.
func runConfigGet(d Deps, key string, reveal bool) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}
	tree, secrets, err := toTree(cfg, reveal)
	if err != nil {
		return err
	}
	v, ok := lookup(tree, key)
	if !ok {
		return fmt.Errorf("no config key %q (see `gx config show`)", key)
	}
	if touchesSecret(key, secrets) {
		return fmt.Errorf("config key %q holds a secret; pass --reveal to print it", key)
	}
	s, err := formatValue(v)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%s\n", s)
	return nil
}

func runConfigDefaults(d Deps) error {
	b, err := json.MarshalIndent(gxconfig.Default(), "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%s\n", b)
	return nil
}

// runConfigTestNotifications sends a test message as a system message on all
// five channels through the server's router, and prints each channel's
// outcome. With no server (or one too old for the route) it tests Telegram and
// Slack itself and says the other three were skipped. It reports each outcome
// as it goes rather than failing fast, so a Slack success is still visible even
// if Telegram is misconfigured.
func runConfigTestNotifications(c *cobra.Command, d Deps, jsonOut bool) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}
	via, err := callRouter(c, d, func(ctx context.Context, a *apiclient.Admin) (wire.NotifyResult, error) {
		return a.TestNotifications(ctx)
	})
	if err != nil {
		return err
	}
	var failed, configured int
	if via.direct {
		var chat []notify.ChannelOutcome
		chat, failed = testChatDirect(cfg, d.Stderr)
		via.res = directResult(chat)
		for _, o := range chat {
			if o.Outcome != notify.OutcomeUnconfigured {
				configured++
			}
		}
	}
	if jsonOut {
		if err := printJSON(d.Stdout, via.res, via.direct); err != nil {
			return err
		}
	} else {
		for _, ch := range via.res.Channels {
			if ch.Outcome != notify.OutcomeFailed {
				fmt.Fprintf(d.Stdout, "%s: %s\n", ch.Channel, ch.Outcome)
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d notification(s) failed to send", failed, configured)
	}
	return nil
}

// testChatDirect tests Telegram and Slack from this process. A channel whose
// switch is off is not tested, so the test tells the truth about what a real
// message would do. It returns the outcomes and how many sends failed.
func testChatDirect(cfg gxconfig.Config, stderr io.Writer) (out []notify.ChannelOutcome, failed int) {
	out = notify.ChatGate(cfg.Notifications, cfg.NotificationChannels, "")
	tg, slack := cfg.Notifications.Telegram, cfg.Notifications.Slack
	sends := []func() error{
		func() error { return notify.SendTelegramTestMessage(tg.BotToken, tg.ChatID) },
		func() error { return notify.SendSlackTestMessage(slack.WebhookURL) },
	}
	for i := range out {
		if out[i].Outcome != "" {
			continue
		}
		if err := sends[i](); err != nil {
			failed++
			fmt.Fprintf(stderr, "%s: failed: %v\n", out[i].Channel, err)
			out[i].Outcome = notify.OutcomeFailed
		} else {
			out[i].Outcome = notify.OutcomeSent
		}
	}
	return out, failed
}
