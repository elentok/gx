package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"rsc.io/qr"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// qrQuietZone is the blank border, in modules, that scanners need around a code.
const qrQuietZone = 2

func newServerWebTokenCmd(d deps) *cobra.Command {
	var rotate bool
	c := &cobra.Command{
		Use:   "web-token",
		Short: "Print the web UI token, or replace it with --rotate (signs out every browser)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			stateDir, err := d.dirs().StateDir()
			if err != nil {
				return err
			}
			return runWebToken(stateDir, rotate, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&rotate, "rotate", false, "replace the token; the running server drops every old cookie")
	return c
}

func newServerWebURLCmd(d deps) *cobra.Command {
	var showQR bool
	c := &cobra.Command{
		Use:   "web-url",
		Short: "Print the web UI link with its token",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			stateDir, err := d.dirs().StateDir()
			if err != nil {
				return err
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runWebURL(ctx, cl, stateDir, showQR, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&showQR, "qr", false, "print a QR code of the link above it, to open it on a phone")
	return c
}

func runWebToken(stateDir string, rotate bool, w io.Writer) error {
	var token string
	var err error
	if rotate {
		token, err = server.RotateWebToken(stateDir)
	} else {
		token, err = server.ReadWebToken(stateDir)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, token)
	return err
}

func runWebURL(ctx context.Context, c *apiclient.Client, stateDir string, showQR bool, w io.Writer) error {
	hs, err := c.Handshake(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if hs.WebURL == "" {
		return errors.New("the web UI is off: set server.web.listen in the config and restart the server")
	}
	token, err := server.ReadWebToken(stateDir)
	if err != nil {
		return err
	}
	url := hs.WebURL + "/?gx_token=" + token
	if showQR {
		art, err := renderQR(url)
		if err != nil {
			return err
		}
		if _, err = io.WriteString(w, art); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w, url)
	return err
}

// renderQR draws the code with half-block characters, so two modules share one
// text row and the code stays roughly square.
func renderQR(text string) (string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", err
	}
	dark := func(x, y int) bool {
		x, y = x-qrQuietZone, y-qrQuietZone
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	size := code.Size + 2*qrQuietZone
	var b strings.Builder
	for y := 0; y < size; y += 2 {
		for x := 0; x < size; x++ {
			switch top, bottom := dark(x, y), dark(x, y+1); {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
