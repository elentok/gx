// Package config is `gx config` (show, edit, defaults, test-notifications) and
// `gx notify`.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	gxconfig "github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
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
	cmd.AddCommand(
		&cobra.Command{
			Use:   "edit",
			Short: "open the config file in $EDITOR (creates it if missing)",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runEditConfig(d)
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "print the effective (merged) config as JSON",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigShow(d)
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
		&cobra.Command{
			Use:   "test-notifications",
			Short: "send a test message to every configured notification service",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return runConfigTestNotifications(d)
			},
		},
	)
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

func runConfigShow(d Deps) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Stdout, "%s\n", b)
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

// runConfigTestNotifications sends a fixed test message to every notification
// service that has credentials configured, reporting each one's outcome as
// it goes rather than failing fast, so a Slack success is still visible even
// if Telegram is misconfigured.
func runConfigTestNotifications(d Deps) error {
	cfg, err := d.LoadConfig()
	if err != nil {
		return err
	}

	configured := 0
	failed := 0

	if cfg.Notifications.Telegram.BotToken != "" {
		configured++
		if err := notify.SendTelegramTestMessage(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID); err != nil {
			failed++
			fmt.Fprintf(d.Stderr, "telegram: failed: %v\n", err)
		} else {
			fmt.Fprintf(d.Stdout, "telegram: sent\n")
		}
	}

	if cfg.Notifications.Slack.WebhookURL != "" {
		configured++
		if err := notify.SendSlackTestMessage(cfg.Notifications.Slack.WebhookURL); err != nil {
			failed++
			fmt.Fprintf(d.Stderr, "slack: failed: %v\n", err)
		} else {
			fmt.Fprintf(d.Stdout, "slack: sent\n")
		}
	}

	if configured == 0 {
		fmt.Fprintf(d.Stdout, "no notification service configured (see `gx config edit`)\n")
		return nil
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d notification(s) failed to send", failed, configured)
	}
	return nil
}
