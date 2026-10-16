package harness

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
)

// ProgramReport is one OSC 7501 Program Status Protocol report:
// ESC ] 7501 ; state=<s>:app=<app>[:id=<task>][:kind=<k>][:progress=<n>][:title=<b64>][:msg=<b64>] BEL.
type ProgramReport struct {
	// State is working, done, idle, blocked or clear (sent on exit).
	State string
	App   string
	// TaskID is set on a per-task report.
	TaskID string
	// Kind is what a blocked program waits on: permission or question.
	Kind string
	// Progress is 0-100, or -1 when the report has none.
	Progress int
	// Title and Message are decoded from base64.
	Title   string
	Message string
}

const osc7501 = "\x1b]7501;"

// ProgramStatusProbe asks the terminal whether it speaks OSC 7501; the
// terminal answers with the same sequence.
const ProgramStatusProbe = osc7501 + "?\a"

// maxOSC7501 bounds an unterminated report the parser holds on to, so a
// stray prefix can't grow the buffer forever.
const maxOSC7501 = 4096

// ProgramStatusParser picks OSC 7501 probes and reports out of a program's
// raw output, fed in chunks as it is read. Everything else is skipped.
type ProgramStatusParser struct {
	pending []byte
}

// Feed parses the next chunk, returning how many probes and which reports it
// completed. A sequence cut off at the chunk's end completes on a later Feed.
func (p *ProgramStatusParser) Feed(chunk []byte) (probes int, reports []ProgramReport) {
	data := append(p.pending, chunk...)
	p.pending = nil
	for {
		i := bytes.Index(data, []byte(osc7501))
		if i < 0 {
			p.pending = bytes.Clone(data[len(data)-partialPrefix(data):])
			return probes, reports
		}
		body := data[i+len(osc7501):]
		end, next := oscEnd(body)
		if next < 0 {
			if len(body) <= maxOSC7501 {
				p.pending = bytes.Clone(data[i:])
				return probes, reports
			}
			data = body
			continue
		}
		data = body[next:]
		if end < 0 {
			continue
		}
		payload := string(body[:end])
		if payload == "?" {
			probes++
		} else if r, ok := parseProgramReport(payload); ok {
			reports = append(reports, r)
		}
	}
}

// oscEnd finds the terminator (BEL or ST) of an OSC body: end is where the
// payload stops and next is past the terminator. next is -1 while the body
// is unterminated. An ESC that starts anything but ST aborts the sequence,
// which then has end -1 and next at that ESC.
func oscEnd(body []byte) (end, next int) {
	i := bytes.IndexAny(body, "\a\x1b")
	switch {
	case i < 0:
		return -1, -1
	case body[i] == '\a':
		return i, i + 1
	case i+1 == len(body):
		return -1, -1
	case body[i+1] == '\\':
		return i, i + 2
	}
	return -1, i
}

// partialPrefix is the length of data's longest suffix that could begin an
// OSC 7501 sequence.
func partialPrefix(data []byte) int {
	for n := min(len(osc7501)-1, len(data)); n > 0; n-- {
		if bytes.HasSuffix(data, []byte(osc7501[:n])) {
			return n
		}
	}
	return 0
}

func parseProgramReport(payload string) (ProgramReport, bool) {
	r := ProgramReport{Progress: -1}
	for field := range strings.SplitSeq(payload, ":") {
		key, value, _ := strings.Cut(field, "=")
		switch key {
		case "state":
			r.State = value
		case "app":
			r.App = value
		case "id":
			r.TaskID = value
		case "kind":
			r.Kind = value
		case "progress":
			if n, err := strconv.Atoi(value); err == nil {
				r.Progress = n
			}
		case "title":
			r.Title = decodeBase64(value)
		case "msg":
			r.Message = decodeBase64(value)
		}
	}
	return r, r.State != ""
}

// decodeBase64 accepts padded and unpadded base64, and drops a value that is
// neither.
func decodeBase64(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, _ = base64.RawStdEncoding.DecodeString(s)
	}
	return string(b)
}
