package kittygraphics

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// chunkSize is the maximum number of base64 bytes the kitty graphics protocol
// allows per APC escape sequence; larger payloads must be split across
// multiple chunked sequences (the "m=1"/"m=0" continuation keys).
const chunkSize = 4096

const (
	apcStart = "\033_G"
	apcEnd   = "\033\\"
)

// EncodePlacement returns the raw bytes to write to the terminal to transmit
// imageBytes (a PNG-encoded image) and display it as a placement identified
// by id, scaled to span exactly spanCols by spanRows terminal cells.
//
// Large payloads are split across multiple chunked APC sequences per the
// protocol's "m" continuation key. When capability.TmuxPassthrough is set,
// every sequence is wrapped in the tmux DCS passthrough envelope.
func EncodePlacement(capability Capability, id uint32, imageBytes []byte, spanCols, spanRows int) []byte {
	payload := make([]byte, base64.StdEncoding.EncodedLen(len(imageBytes)))
	base64.StdEncoding.Encode(payload, imageBytes)

	var out bytes.Buffer
	for offset := 0; offset < len(payload) || offset == 0; offset += chunkSize {
		end := min(offset+chunkSize, len(payload))
		chunk := payload[offset:end]
		more := 0
		if end < len(payload) {
			more = 1
		}

		var controlData string
		if offset == 0 {
			controlData = fmt.Sprintf("a=T,f=100,i=%d,c=%d,r=%d,q=2,m=%d", id, spanCols, spanRows, more)
		} else {
			controlData = fmt.Sprintf("m=%d", more)
		}

		out.Write(encodeAPC(capability, controlData, chunk))

		if len(payload) == 0 {
			break
		}
	}

	return out.Bytes()
}

// EncodeClear returns the raw bytes to write to the terminal to delete the
// placement identified by id (and the underlying image data), wrapped in the
// tmux DCS passthrough envelope when capability.TmuxPassthrough is set.
func EncodeClear(capability Capability, id uint32) []byte {
	controlData := fmt.Sprintf("a=d,d=I,i=%d,q=2", id)
	return encodeAPC(capability, controlData, nil)
}

// encodeAPC builds a single kitty graphics APC escape sequence
// ("\033_G<control data>;<base64 payload>\033\\"), wrapping it in the tmux
// DCS passthrough envelope when the capability requires it.
func encodeAPC(capability Capability, controlData string, payload []byte) []byte {
	var seq bytes.Buffer
	seq.WriteString(apcStart)
	seq.WriteString(controlData)
	seq.WriteByte(';')
	seq.Write(payload)
	seq.WriteString(apcEnd)

	if capability.TmuxPassthrough {
		return wrapTmuxPassthrough(seq.Bytes())
	}
	return seq.Bytes()
}

// wrapTmuxPassthrough wraps seq in the tmux DCS passthrough envelope
// ("\033Ptmux;<seq with ESC doubled>\033\\"), which tmux unwraps and forwards
// verbatim to the host terminal. Every ESC byte inside the sequence must be
// doubled, since a lone ESC would terminate the DCS string early.
func wrapTmuxPassthrough(seq []byte) []byte {
	var out bytes.Buffer
	out.WriteString("\033Ptmux;")
	for _, b := range seq {
		if b == '\033' {
			out.WriteByte('\033')
		}
		out.WriteByte(b)
	}
	out.WriteString("\033\\")
	return out.Bytes()
}
