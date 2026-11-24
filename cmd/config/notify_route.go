package config

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/wire"
)

// outcomeSkipped is the direct fallback's outcome for the three channels only
// the server can reach. The server never answers it.
const outcomeSkipped = "skipped"

// routedVia is the result of one router call: the server's answer, or none.
type routedVia struct {
	res wire.NotifyResult
	// direct is true when no usable server answered and the caller must send itself.
	direct bool
}

// callRouter asks the server to send. No server, or one too old to have the
// route, is not an error: the caller falls back to sending itself.
func callRouter(c *cobra.Command, d Deps, call func(context.Context, *apiclient.Admin) (wire.NotifyResult, error)) (routedVia, error) {
	cl, err := d.Dial()()
	if err != nil {
		return routedVia{}, err
	}
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := call(ctx, cl.Admin)
	switch {
	case err == nil:
		return routedVia{res: res}, nil
	case apiclient.IsNotRunning(err) || failedDial(err) || apiclient.IsUnknownRoute(err):
		return routedVia{direct: true}, nil
	}
	return routedVia{}, fmt.Errorf("server write failed: %w", err)
}

// failedDial reports whether the connection never opened. A socket path too
// long for the OS fails that way (EINVAL) and no server can listen there either.
func failedDial(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

// directResult is the fallback's result in the router's shape: the two chat
// channels as sent, and the other three skipped.
func directResult(chat []notify.ChannelOutcome) wire.NotifyResult {
	byChannel := map[string]string{}
	for _, o := range chat {
		byChannel[o.Channel] = o.Outcome
	}
	var res wire.NotifyResult
	for _, ch := range []string{notify.ChannelDesktop, notify.ChannelWebToast, notify.ChannelTelegram, notify.ChannelSlack, notify.ChannelPush} {
		res.Channels = append(res.Channels, wire.NotifyChannel{Channel: ch, Outcome: cmp.Or(byChannel[ch], outcomeSkipped)})
	}
	return res
}

// printJSON prints res stamped with where it came from.
func printJSON(w io.Writer, res wire.NotifyResult, direct bool) error {
	if !direct {
		return cmdbase.EncodeProvenanceRefusal(w, res, false)
	}
	return cmdbase.EncodeProvenance(w, res, cmdbase.ViaDirect, cmdbase.CallerActor(os.Getwd))
}

// sentTo lists the channels that sent, in order.
func sentTo(res wire.NotifyResult) []string {
	var sent []string
	for _, ch := range res.Channels {
		if ch.Outcome == notify.OutcomeSent {
			sent = append(sent, ch.Channel)
		}
	}
	return sent
}

// skippedLine names every channel that did not send and why, "" when none.
func skippedLine(res wire.NotifyResult) string {
	var parts []string
	for _, ch := range res.Channels {
		if ch.Outcome != notify.OutcomeSent {
			parts = append(parts, ch.Channel+" ("+ch.Outcome+")")
		}
	}
	return strings.Join(parts, ", ")
}
