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

// Refusable is a web payload that can carry a refusal, so one runner serves every web command.
type Refusable[T any] interface {
	Refusal() (refused bool, reason, message string)
	Refuse(reason, message string) T
}

// webClient dials the server and returns the command's context.
func webClient(d deps, c *cobra.Command) (*apiclient.Client, context.Context, error) {
	cl, err := d.dial()()
	if err != nil {
		return nil, nil, err
	}
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return cl, ctx, nil
}

// runWebCall runs one web socket call and prints its result. A server that is
// not running comes back as a refusal, like the other socket reads; failure
// prefixes any other error.
func runWebCall[T Refusable[T]](ctx context.Context, call func(context.Context) (T, error), failure string, jsonOut bool, w io.Writer, print func(T) error) error {
	res, err := call(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		res = res.Refuse(server.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
	case err != nil:
		return fmt.Errorf("%s: %w", failure, err)
	}
	if jsonOut {
		if err := encodeProvenance(w, res, viaServer, callerActor(os.Getwd)); err != nil {
			return err
		}
	}
	if refused, reason, message := res.Refusal(); refused {
		return fmt.Errorf("refused (%s): %s", reason, message)
	}
	if jsonOut {
		return nil
	}
	return print(res)
}

func newServerWebURLCmd(d deps) *cobra.Command {
	var showQR, jsonOut bool
	c := &cobra.Command{
		Use:   "web-url",
		Short: "Print the web UI link, with the Setup code while setup is pending",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebURL(ctx, cl, showQR, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().BoolVar(&showQR, "qr", false, "print a QR code of the link above it, to open it on a phone")
	return c
}

// webURLResult is the `gx server web-url --json` payload.
type webURLResult struct {
	server.WebInfo
	// URL is the setup link while setup is pending, else the base URL.
	URL string `json:"url,omitempty"`
}

func (webURLResult) Refuse(reason, message string) webURLResult {
	return webURLResult{WebInfo: server.WebInfo{}.Refuse(reason, message)}
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
	read := func(ctx context.Context) (webURLResult, error) {
		info, err := c.WebInfo(ctx)
		res := webURLResult{WebInfo: info}
		if err == nil && !info.Refused {
			res.URL = info.BaseURL
			if info.SetupPending {
				res.URL = setupLink(info.BaseURL, info.SetupCode)
			}
		}
		return res, err
	}
	return runWebCall(ctx, read, "server read failed", jsonOut, w, func(res webURLResult) error {
		if showQR {
			art, err := renderQR(res.URL)
			if err != nil {
				return err
			}
			if _, err = io.WriteString(w, art); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, res.URL); err != nil {
			return err
		}
		if res.SetupCode == "" {
			return nil
		}
		_, err := fmt.Fprintln(w, setupCodeLine(res.SetupCode, res.SetupExpires))
		return err
	})
}

func newServerWebNewRootCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-new-root",
		Short: "Make a new web root certificate and serve it at once",
		Long:  "Makes a new root from the current allowed hosts and re-issues the leaf from it.\nDevices must trust the new root again; web logins are kept.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebNewRoot(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebNewRoot(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	return runWebCall(ctx, c.WebNewRoot, "server write failed", jsonOut, w, func(info server.WebInfo) error {
		_, err := fmt.Fprintln(w, "new root fingerprint: "+info.Fingerprint)
		return err
	})
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
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebReset(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "reset without asking")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebReset(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	return runWebCall(ctx, c.WebReset, "server write failed", jsonOut, w, printSetupCode(w))
}

// printSetupCode prints the Setup code line of a reset result.
func printSetupCode(w io.Writer) func(server.WebInfo) error {
	return func(info server.WebInfo) error {
		_, err := fmt.Fprintln(w, setupCodeLine(info.SetupCode, info.SetupExpires))
		return err
	}
}

func newServerWebLoginsCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-logins",
		Short: "List the web logins with their name, created time, last use and last IP",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebLogins(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebLogins(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	return runWebCall(ctx, c.WebLogins, "server request failed", jsonOut, w, func(res server.WebLoginsResult) error {
		if len(res.Logins) == 0 {
			_, err := fmt.Fprintln(w, "no web logins")
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
	})
}

func newServerWebRevokeCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-revoke <id>",
		Short: "Sign one web login out by id and close its event streams",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebRevoke(ctx, cl, args[0], jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebRevoke(ctx context.Context, c *apiclient.Client, id string, jsonOut bool, w io.Writer) error {
	revoke := func(ctx context.Context) (server.WebLoginsResult, error) { return c.WebRevokeLogin(ctx, id) }
	return runWebCall(ctx, revoke, "server request failed", jsonOut, w, func(server.WebLoginsResult) error {
		_, err := fmt.Fprintln(w, "revoked web login "+id)
		return err
	})
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
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
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
	set := func(ctx context.Context) (server.WebLoginsResult, error) { return c.WebSetPassword(ctx, password) }
	return runWebCall(ctx, set, "server request failed", jsonOut, w, func(server.WebLoginsResult) error {
		_, err := fmt.Fprintln(w, "web password changed; every web login is signed out")
		return err
	})
}

func newServerWebResetAuthenticatorCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "web-reset-authenticator",
		Short: "Clear the web authenticator, sign every browser out and print a new Setup code",
		Long:  "Clears the TOTP secret and signs every web login out. The next web login needs the Setup code and the password, then adds a new authenticator in the browser.\nNothing here prints a TOTP secret.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return runWebResetAuthenticator(ctx, cl, jsonOut, c.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runWebResetAuthenticator(ctx context.Context, c *apiclient.Client, jsonOut bool, w io.Writer) error {
	return runWebCall(ctx, c.WebResetAuthenticator, "server write failed", jsonOut, w, printSetupCode(w))
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
