package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
)

type serverLogsOpts struct {
	Follow bool
	JSON   bool
	Level  string
}

const logFollowInterval = 200 * time.Millisecond

// runServerLogs prints the server's slog JSON file, optionally following it
// until ctx is done.
func runServerLogs(ctx context.Context, path string, opts serverLogsOpts, w io.Writer) error {
	var min slog.Level
	if opts.Level != "" {
		if err := min.UnmarshalText([]byte(opts.Level)); err != nil {
			return fmt.Errorf("invalid --level %q (want debug, info, warn or error)", opts.Level)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no server log yet (%s)", path)
		}
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var offset int64
	var partial string
	for {
		chunk, err := r.ReadString('\n')
		offset += int64(len(chunk))
		if err == nil {
			if werr := writeLogLine(w, partial+strings.TrimSuffix(chunk, "\n"), min, opts.JSON); werr != nil {
				return werr
			}
			partial = ""
			continue
		}
		if err != io.EOF {
			return err
		}
		partial += chunk
		if !opts.Follow {
			if partial != "" {
				return writeLogLine(w, partial, min, opts.JSON)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(logFollowInterval):
		}
		// A shrunk file means the server rotated it; start over on the new one.
		if info, serr := os.Stat(path); serr == nil && info.Size() < offset {
			f.Close()
			if f, err = os.Open(path); err != nil {
				return err
			}
			r.Reset(f)
			offset, partial = 0, ""
		}
	}
}

func writeLogLine(w io.Writer, line string, min slog.Level, rawJSON bool) error {
	if line == "" {
		return nil
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		// Not slog output (e.g. a panic trace): never hide it.
		_, werr := fmt.Fprintln(w, line)
		return werr
	}
	var lvl slog.Level
	if s, _ := rec["level"].(string); s != "" {
		_ = lvl.UnmarshalText([]byte(s))
	}
	if lvl < min {
		return nil
	}
	if rawJSON {
		_, err := fmt.Fprintln(w, line)
		return err
	}
	_, err := fmt.Fprintln(w, prettyLogRecord(rec))
	return err
}

func prettyLogRecord(rec map[string]any) string {
	ts, _ := rec[slog.TimeKey].(string)
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		ts = t.Local().Format("2006-01-02 15:04:05")
	}
	level, _ := rec[slog.LevelKey].(string)
	msg, _ := rec[slog.MessageKey].(string)
	var attrs []string
	for k, v := range rec {
		switch k {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey:
			continue
		}
		attrs = append(attrs, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(attrs)
	return strings.TrimSpace(fmt.Sprintf("%s %-5s %s %s", ts, level, msg, strings.Join(attrs, " ")))
}
