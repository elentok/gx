package claudedoctor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
)

// PtyLiveLabel is the agent label PtyLive starts under p.Root.
const PtyLiveLabel = "doctor"

// PtyLive starts one claude through the PTY runner in a fresh cwd, runs
// drive against it, then stops it. It records every screen change and every
// pid status change from launch on, so the trust dialog Start answers itself
// is still captured.
func PtyLive(p *nativerunner.Pty, cwd string, drive func(agentrunner.Session) error) (Recording, error) {
	rec := &ptyRecorder{dir: filepath.Join(p.Root, PtyLiveLabel)}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			rec.sample()
			select {
			case <-done:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	stopRecording := func() Recording {
		close(done)
		wg.Wait()
		rec.sample()
		return rec.Recording
	}

	s, err := p.Start(agentrunner.StartOptions{Label: PtyLiveLabel, Epic: "doctor", Cwd: cwd, Kind: "claude"})
	if err != nil {
		stopRecording()
		return Recording{}, err
	}
	err = drive(s)
	out := stopRecording()
	if stopErr := p.Stop(s); err == nil {
		err = stopErr
	}
	return out, err
}

type ptyRecorder struct {
	dir    string
	holder *nativerunner.HolderClient
	Recording
	lastStatus string
}

func (r *ptyRecorder) sample() {
	if r.holder == nil {
		h, err := nativerunner.DialHolder(r.dir)
		if err != nil {
			return
		}
		r.holder = h
	}
	if rows, _, err := r.holder.Screen(0); err == nil {
		frame := strings.Join(rows, "\n")
		if n := len(r.Frames); strings.TrimSpace(frame) != "" && (n == 0 || r.Frames[n-1] != frame) {
			r.Frames = append(r.Frames, frame)
		}
	}
	r.snapshot()
}

// snapshot keeps the raw pid status file each time its status or waitingFor
// changes; the status-less file claude writes first is not a snapshot.
func (r *ptyRecorder) snapshot() {
	sessions, err := nativerunner.ClaudeSessionsDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(sessions, strconv.Itoa(r.holder.ChildPID())+".json"))
	var line bytes.Buffer
	if err != nil || json.Compact(&line, data) != nil {
		return
	}
	data = line.Bytes()
	s, err := nativerunner.ParsePidStatus(data)
	if err != nil || s.Status == nativerunner.ClaudeStarting {
		return
	}
	if key := s.Status + "\x00" + s.WaitingFor; key != r.lastStatus {
		r.lastStatus = key
		r.Snapshots = append(r.Snapshots, json.RawMessage(data))
	}
}
