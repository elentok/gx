package kittygraphics

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
)

func TestEncodePlacement(t *testing.T) {
	plain := Capability{Supported: true}
	tmux := Capability{Supported: true, TmuxPassthrough: true}

	cases := []struct {
		name string

		capability Capability
		id         uint32
		imageBytes []byte
		spanCols   int
		spanRows   int

		want string
	}{
		{
			name:       "small image, plain terminal: single chunk, m=0",
			capability: plain,
			id:         1,
			imageBytes: []byte("PNG-bytes"),
			spanCols:   10,
			spanRows:   5,
			want:       "\033_Ga=T,f=100,i=1,c=10,r=5,q=2,m=0;UE5HLWJ5dGVz\033\\",
		},
		{
			name:       "small image, tmux passthrough: single chunk wrapped",
			capability: tmux,
			id:         1,
			imageBytes: []byte("PNG-bytes"),
			spanCols:   10,
			spanRows:   5,
			want:       wrapTmuxString("\033_Ga=T,f=100,i=1,c=10,r=5,q=2,m=0;UE5HLWJ5dGVz\033\\"),
		},
		{
			name:       "empty image: still emits a single placement chunk",
			capability: plain,
			id:         7,
			imageBytes: []byte{},
			spanCols:   1,
			spanRows:   1,
			want:       "\033_Ga=T,f=100,i=7,c=1,r=1,q=2,m=0;\033\\",
		},
		{
			name:       "large image: split across chunked sequences",
			capability: plain,
			id:         2,
			imageBytes: bytes.Repeat([]byte{0xAB}, 4000),
			spanCols:   40,
			spanRows:   20,
			want:       largePlacementWant(plain, 2, 40, 20, bytes.Repeat([]byte{0xAB}, 4000)),
		},
		{
			name:       "large image over tmux: every chunk wrapped",
			capability: tmux,
			id:         2,
			imageBytes: bytes.Repeat([]byte{0xAB}, 4000),
			spanCols:   40,
			spanRows:   20,
			want:       largePlacementWant(tmux, 2, 40, 20, bytes.Repeat([]byte{0xAB}, 4000)),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EncodePlacement(c.capability, c.id, c.imageBytes, c.spanCols, c.spanRows)
			if string(got) != c.want {
				t.Fatalf("EncodePlacement() =\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestEncodeClear(t *testing.T) {
	cases := []struct {
		name       string
		capability Capability
		id         uint32
		want       string
	}{
		{
			name:       "plain terminal",
			capability: Capability{Supported: true},
			id:         3,
			want:       "\033_Ga=d,d=I,i=3,q=2;\033\\",
		},
		{
			name:       "tmux passthrough wraps the sequence",
			capability: Capability{Supported: true, TmuxPassthrough: true},
			id:         3,
			want:       wrapTmuxString("\033_Ga=d,d=I,i=3,q=2;\033\\"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EncodeClear(c.capability, c.id)
			if string(got) != c.want {
				t.Fatalf("EncodeClear() =\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

// wrapTmuxString applies the tmux DCS passthrough envelope to a plain escape
// sequence string, doubling ESC bytes, mirroring wrapTmuxPassthrough exactly
// (re-implemented here so the test fails if the production wrapping changes
// shape, not just because it calls the same helper).
func wrapTmuxString(seq string) string {
	var b strings.Builder
	b.WriteString("\033Ptmux;")
	for _, r := range []byte(seq) {
		if r == '\033' {
			b.WriteByte('\033')
		}
		b.WriteByte(r)
	}
	b.WriteString("\033\\")
	return b.String()
}

// largePlacementWant builds the expected byte-exact output for a multi-chunk
// placement: first chunk carries the full control data with m=1, subsequent
// chunks carry only "m=<more>", each independently wrapped when needed.
func largePlacementWant(capability Capability, id uint32, spanCols, spanRows int, imageBytes []byte) string {
	payload := base64.StdEncoding.EncodeToString(imageBytes)

	var b strings.Builder
	for offset := 0; offset < len(payload); offset += chunkSize {
		end := min(offset+chunkSize, len(payload))
		chunk := payload[offset:end]
		more := 0
		if end < len(payload) {
			more = 1
		}

		var seq string
		if offset == 0 {
			seq = "\033_Ga=T,f=100,i=2,c=" + strconv.Itoa(spanCols) + ",r=" + strconv.Itoa(spanRows) + ",q=2,m=" + strconv.Itoa(more) + ";" + chunk + "\033\\"
		} else {
			seq = "\033_Gm=" + strconv.Itoa(more) + ";" + chunk + "\033\\"
		}

		if capability.TmuxPassthrough {
			seq = wrapTmuxString(seq)
		}
		b.WriteString(seq)
	}
	return b.String()
}
