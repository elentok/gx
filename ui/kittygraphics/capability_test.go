package kittygraphics

import "testing"

func TestDetectSupport(t *testing.T) {
	winSize80x24 := WinSize{Cols: 80, Rows: 24, PixelWidth: 800, PixelHeight: 480}

	cases := []struct {
		name string

		env       map[string]string
		winSize   WinSize
		winSizeOK bool
		probeResp string
		probeOK   bool

		want Capability
	}{
		{
			name:      "plain terminal: unsupported, no probe",
			env:       map[string]string{},
			winSize:   winSize80x24,
			winSizeOK: true,
			want:      Capability{Supported: false, TmuxPassthrough: false},
		},
		{
			name:      "direct kitty: supported with pixel-per-cell, no passthrough",
			env:       map[string]string{"KITTY_WINDOW_ID": "12"},
			winSize:   winSize80x24,
			winSizeOK: true,
			want: Capability{
				Supported:       true,
				PixelsPerCol:    10,
				PixelsPerRow:    20,
				TmuxPassthrough: false,
			},
		},
		{
			name:      "kitty with remote control: supported, no passthrough",
			env:       map[string]string{"KITTY_LISTEN_ON": "unix:/tmp/mykitty-1"},
			winSize:   winSize80x24,
			winSizeOK: true,
			want: Capability{
				Supported:       true,
				PixelsPerCol:    10,
				PixelsPerRow:    20,
				TmuxPassthrough: false,
			},
		},
		{
			// ui.DetectTerminalFrom treats $KITTY_WINDOW_ID as authoritative over
			// $TMUX (gx is considered to be running directly in kitty), so no
			// passthrough wrapping is needed even though $TMUX is also set.
			name:      "kitty env wins over tmux env: direct, no passthrough needed",
			env:       map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0", "KITTY_WINDOW_ID": "12"},
			winSize:   winSize80x24,
			winSizeOK: true,
			want: Capability{
				Supported:       true,
				PixelsPerCol:    10,
				PixelsPerRow:    20,
				TmuxPassthrough: false,
			},
		},
		{
			name:      "non-kitty in tmux, probe confirms host is kitty: passthrough viable",
			env:       map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			winSize:   winSize80x24,
			winSizeOK: true,
			probeResp: "\033_Gi=1;OK\033\\",
			probeOK:   true,
			want: Capability{
				Supported:       true,
				PixelsPerCol:    10,
				PixelsPerRow:    20,
				TmuxPassthrough: true,
			},
		},
		{
			name:      "non-kitty in tmux, probe says host is not kitty: unsupported",
			env:       map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			winSize:   winSize80x24,
			winSizeOK: true,
			probeResp: "",
			probeOK:   false,
			want:      Capability{Supported: false, TmuxPassthrough: false},
		},
		{
			name:      "non-kitty in tmux, probe responds but isn't a kitty OK: unsupported",
			env:       map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			winSize:   winSize80x24,
			winSizeOK: true,
			probeResp: "garbage",
			probeOK:   true,
			want:      Capability{Supported: false, TmuxPassthrough: false},
		},
		{
			name:      "kitty remote control env wins over tmux env: direct, no passthrough needed",
			env:       map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0", "KITTY_LISTEN_ON": "unix:/tmp/mykitty-1"},
			winSize:   winSize80x24,
			winSizeOK: true,
			want: Capability{
				Supported:       true,
				PixelsPerCol:    10,
				PixelsPerRow:    20,
				TmuxPassthrough: false,
			},
		},
		{
			name:      "supported but winsize query fails: pixel-per-cell stays zero",
			env:       map[string]string{"KITTY_WINDOW_ID": "12"},
			winSize:   WinSize{},
			winSizeOK: false,
			want:      Capability{Supported: true, TmuxPassthrough: false},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(key string) string { return c.env[key] }

			queryWinSize := func() (WinSize, bool) { return c.winSize, c.winSizeOK }

			probeCalls := 0
			probe := func(query string) (string, bool) {
				probeCalls++
				return c.probeResp, c.probeOK
			}

			got := DetectSupport(getenv, queryWinSize, probe)
			if got != c.want {
				t.Fatalf("DetectSupport() = %+v, want %+v", got, c.want)
			}

			directKitty := c.env["KITTY_WINDOW_ID"] != "" || c.env["KITTY_LISTEN_ON"] != ""
			inTmux := c.env["TMUX"] != ""
			wantProbeCall := inTmux && !directKitty
			if (probeCalls > 0) != wantProbeCall {
				t.Fatalf("probe called = %v, want called = %v", probeCalls > 0, wantProbeCall)
			}
		})
	}
}

func TestDetectSupportProbeReceivesTmuxWrappedQuery(t *testing.T) {
	getenv := func(key string) string {
		if key == "TMUX" {
			return "/tmp/tmux-1000/default,1,0"
		}
		return ""
	}
	queryWinSize := func() (WinSize, bool) { return WinSize{}, false }

	var gotQuery string
	probe := func(query string) (string, bool) {
		gotQuery = query
		return "\033_Gi=1;OK\033\\", true
	}

	DetectSupport(getenv, queryWinSize, probe)

	wantQuery := "\033Ptmux;\033\033_Gi=1,a=q\033\033\\\033\\"
	if gotQuery != wantQuery {
		t.Fatalf("probe query = %q, want %q", gotQuery, wantQuery)
	}
}
