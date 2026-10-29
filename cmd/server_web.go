package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"rsc.io/qr"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// qrQuietZone is the blank border, in modules, that scanners need around a code.
const qrQuietZone = 2

func newServerWebURLCmd(d deps) *cobra.Command {
	var showQR, jsonOut bool
	c := &cobra.Command{
		Use:   "web-url",
		Short: "Print the web UI link, with the Setup code while setup is pending",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runWebURL(ctx, cl, showQR, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().BoolVar(&showQR, "qr", false, "print a QR code of the link above it, to open it on a phone")
	return c
}

// webInfoOrRefusal reads the web info route; a server that is not running comes
// back as a refusal, like the other socket reads.
func webInfoOrRefusal(ctx context.Context, c *apiclient.Client) (server.WebInfo, error) {
	info, err := c.WebInfo(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		return server.WebInfo{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, nil
	case err != nil:
		return info, fmt.Errorf("server read failed: %w", err)
	}
	return info, nil
}

// webURLResult is the `gx server web-url --json` payload.
type webURLResult struct {
	server.WebInfo
	// URL is the setup link while setup is pending, else the base URL.
	URL string `json:"url,omitempty"`
}

// setupLink is the link that opens the web page's setup step with the code filled in.
func setupLink(baseURL, code string) string {
	return baseURL + "/#/setup?code=" + code
}

// setupCodeLine is the Setup code and the time it stops working, for text output.
func setupCodeLine(code string, expires time.Time) string {
	return "setup code: " + code + " (expires " + expires.Local().Format("15:04") + ")"
}

func runWebURL(ctx context.Context, c *apiclient.Client, showQR, jsonOut bool, w io.Writer) error {
	info, err := webInfoOrRefusal(ctx, c)
	if err != nil {
		return err
	}
	res := webURLResult{WebInfo: info}
	if !info.Refused {
		res.URL = info.BaseURL
		if info.SetupPending {
			res.URL = setupLink(info.BaseURL, info.SetupCode)
		}
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if info.Refused {
		return fmt.Errorf("refused (%s): %s", info.Reason, info.Message)
	}
	if jsonOut {
		return nil
	}
	url := res.URL
	if showQR {
		art, err := renderQR(url)
		if err != nil {
			return err
		}
		if _, err = io.WriteString(w, art); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintln(w, url); err != nil {
		return err
	}
	if info.SetupPending {
		_, err = fmt.Fprintln(w, setupCodeLine(info.SetupCode, info.SetupExpires))
	}
	return err
}

func newServerWebNewRootCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-new-root",
		Short: "Make a new web root certificate and serve it at once",
		Long:  "Makes a new root from the current allowed hosts and re-issues the leaf from it.\nDevices must trust the new root again; web logins are kept.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runWebNewRoot(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebNewRoot(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	info, err := c.WebNewRoot(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		info = server.WebInfo{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := encodeProvenance(w, info, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if info.Refused {
		return fmt.Errorf("refused (%s): %s", info.Reason, info.Message)
	}
	if jsonOut {
		return nil
	}
	_, err = fmt.Fprintln(w, "new root fingerprint: "+info.Fingerprint)
	return err
}

func newServerWebResetCmd(d deps) *cobra.Command {
	var yes, jsonOut bool
	c := &cobra.Command{
		Use:   "web-reset",
		Short: "Clear the web password, authenticator and web logins, and print a new Setup code",
		Long:  "Clears the password, the TOTP secret, every web login and the rate limits, and signs every browser out.\nAsks first on a terminal; without one, --yes is required.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !yes {
				if !isTerminal(d) {
					return errors.New("refusing to reset the web account without a terminal to confirm on; pass --yes")
				}
				fmt.Fprint(d.stdout, "Clear the web password, authenticator and every web login? [y/N] ")
				line, _ := bufio.NewReader(d.stdin).ReadString('\n')
				if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
					fmt.Fprintln(d.stdout, "cancelled; nothing changed")
					return nil
				}
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runWebReset(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "reset without asking")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebReset(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	info, err := c.WebReset(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		info = server.WebInfo{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := encodeProvenance(w, info, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if info.Refused {
		return fmt.Errorf("refused (%s): %s", info.Reason, info.Message)
	}
	if jsonOut {
		return nil
	}
	_, err = fmt.Fprintln(w, setupCodeLine(info.SetupCode, info.SetupExpires))
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
