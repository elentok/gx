package nativerunner

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
)

// Died maps err from a holder call for the session label, as Pty does.
func Died(p *Pty, label string, err error) error {
	return p.died(&ptySession{session: agentrunner.Session{Label: label}}, err)
}

// RunTestHolder runs a holder with h as its harness.
func RunTestHolder(dir string, h harness.Harness, argv []string) error {
	return runHolder(dir, h, HolderOptions{}, argv)
}

// RunTestSessionHolder runs a holder for the gx session gxSession.
func RunTestSessionHolder(dir string, h harness.Harness, gxSession string, argv []string) error {
	return runHolder(dir, h, HolderOptions{GXSession: gxSession}, argv)
}

// PS is the real process table.
var PS ProcessTable = psTable{}

// SetHolderOptionalOps makes holders started by the test advertise ops.
func SetHolderOptionalOps(t *testing.T, ops []string) {
	old := holderOptionalOps
	holderOptionalOps = ops
	t.Cleanup(func() { holderOptionalOps = old })
}

// HolderOptionalOps and HolderDenyTimeout configure a holder the test binary
// runs as a subprocess, which has no *testing.T to restore them.
var (
	HolderOptionalOps = &holderOptionalOps
	HolderDenyTimeout = &holderDenyTimeout
)

// SetHolderAfterExit runs fn in a holder started by the test once its child
// has exited, before the holder winds down and closes holder.sock.
func SetHolderAfterExit(t *testing.T, fn func()) {
	old := holderAfterExit
	holderAfterExit = fn
	t.Cleanup(func() { holderAfterExit = old })
}

// SetHolderDenyTimeout shortens deny-reason's waits for the test.
func SetHolderDenyTimeout(t *testing.T, d time.Duration) {
	old := holderDenyTimeout
	holderDenyTimeout = d
	t.Cleanup(func() { holderDenyTimeout = old })
}

const FrameInterval = defaultFrameInterval

// SetFramePacing sets the attach pacing for the test, so a check can use
// intervals far above any load delay. Call it before attaching.
func SetFramePacing(t *testing.T, interval, echoWindow time.Duration) {
	oldInterval, oldWindow := frameInterval, keyEchoWindow
	frameInterval, keyEchoWindow = interval, echoWindow
	t.Cleanup(func() { frameInterval, keyEchoWindow = oldInterval, oldWindow })
}

// ArmedFrameTimers is how many attach pacing timers are armed.
func ArmedFrameTimers() int64 { return armedFrameTimers.Load() }

// SendOversizedHeader writes the header of a message one byte over the
// stream limit, which writeStreamMessage itself refuses to send.
func SendOversizedHeader(a *HolderAttach) error {
	head := binary.BigEndian.AppendUint32(nil, maxStreamPayload+1)
	_, err := a.conn.Write(append(head, byte(StreamInput)))
	return err
}
