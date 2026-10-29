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

	"github.com/charmbracelet/x/term"
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
	if info.SetupCode != "" {
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

func newServerWebLoginsCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-logins",
		Short: "List the web logins with their name, created time, last use and last IP",
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
			return runWebLogins(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

// webLoginsOrRefusal turns a server that is not running into a refusal.
func webLoginsOrRefusal(res server.WebLoginsResult, err error) (server.WebLoginsResult, error) {
	switch {
	case apiclient.IsNotRunning(err):
		return server.WebLoginsResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, nil
	case err != nil:
		return res, fmt.Errorf("server request failed: %w", err)
	}
	return res, nil
}

func runWebLogins(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	res, err := c.WebLogins(ctx)
	if res, err = webLoginsOrRefusal(res, err); err != nil {
		return err
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if jsonOut {
		return nil
	}
	if len(res.Logins) == 0 {
		_, err = fmt.Fprintln(w, "no web logins")
		return err
	}
	for _, l := range res.Logins {
		last := "never"
		if !l.LastUsed.IsZero() {
			last = l.LastUsed.Local().Format("2006-01-02 15:04")
		}
		if _, err := fmt.Fprintf(w, "%s  %s  created %s  last used %s  from %s\n",
			l.ID, l.Name, l.Created.Local().Format("2006-01-02 15:04"), last, l.LastIP); err != nil {
			return err
		}
	}
	return nil
}

func newServerWebRevokeCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-revoke <id>",
		Short: "Sign one web login out by id and close its event streams",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runWebRevoke(ctx, cl, args[0], jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebRevoke(ctx context.Context, c *apiclient.Client, id string, jsonOut bool, w io.Writer) error {
	res, err := c.WebRevokeLogin(ctx, id)
	if res, err = webLoginsOrRefusal(res, err); err != nil {
		return err
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if jsonOut {
		return nil
	}
	_, err = fmt.Fprintln(w, "revoked web login "+id)
	return err
}

func newServerWebPasswordCmd(d deps) *cobra.Command {
	var jsonOut, fromStdin bool
	c := &cobra.Command{
		Use:   "web-password",
		Short: "Set a new web password without the old one and sign every web login out",
		Long:  "Asks for the new password twice, without echo, on a terminal.\nWithout one, pass --password-stdin and send the password as the first line of stdin.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			password, err := readNewPassword(d, fromStdin)
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
			return runWebPassword(ctx, cl, password, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	c.Flags().BoolVar(&fromStdin, "password-stdin", false, "read the new password from the first line of stdin")
	return c
}

// readNewPassword takes the password from stdin or, on a terminal, prompts for
// it twice without echo and refuses a mismatch.
func readNewPassword(d deps, fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(d.stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	f, ok := d.stdin.(*os.File)
	if !isTerminal(d) || !ok {
		return "", errors.New("no terminal to ask on; pass --password-stdin")
	}
	ask := func(prompt string) (string, error) {
		fmt.Fprint(d.stdout, prompt)
		b, err := term.ReadPassword(f.Fd())
		fmt.Fprintln(d.stdout)
		return string(b), err
	}
	first, err := ask("New web password: ")
	if err != nil {
		return "", err
	}
	again, err := ask("New web password again: ")
	if err != nil {
		return "", err
	}
	if first != again {
		return "", errors.New("the passwords do not match; nothing changed")
	}
	return first, nil
}

func runWebPassword(ctx context.Context, c *apiclient.Client, password string, jsonOut bool, w io.Writer) error {
	res, err := c.WebSetPassword(ctx, password)
	if res, err = webLoginsOrRefusal(res, err); err != nil {
		return err
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if jsonOut {
		return nil
	}
	_, err = fmt.Fprintln(w, "web password changed; every web login is signed out")
	return err
}

func newServerWebResetAuthenticatorCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-reset-authenticator",
		Short: "Clear the web authenticator, sign every browser out and print a new Setup code",
		Long:  "Clears the TOTP secret and signs every web login out. The next web login needs the Setup code and the password, then adds a new authenticator in the browser.\nNothing here prints a TOTP secret.",
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
			return runWebResetAuthenticator(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebResetAuthenticator(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	info, err := c.WebResetAuthenticator(ctx)
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
