package cmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/transcript"
)

func newClaudeSessionPathCmd(d deps) *cobra.Command {
	var grepPattern string

	cmd := &cobra.Command{
		Use:   "session-path <session-id>",
		Short: "locate a Claude Code transcript file by session id",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runClaudeSessionPath(d, args[0], grepPattern)
		},
	}
	cmd.Flags().StringVar(&grepPattern, "grep", "", "print only the located transcript's lines matching this pattern (case-insensitive regex) instead of its path")
	return cmd
}

// runClaudeSessionPath locates sessionID's transcript across every project
// directory (the caller usually doesn't know the slugified cwd Path/PathIn
// need) and either prints its path or, with grepPattern set, filters its
// lines.
func runClaudeSessionPath(d deps, sessionID, grepPattern string) error {
	home, err := d.userHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	matches, err := transcript.FindByID(home, sessionID)
	if err != nil {
		return fmt.Errorf("search for session %s: %w", sessionID, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no transcript found for session %s", sessionID)
	}
	if len(matches) > 1 {
		for _, m := range matches {
			fmt.Fprintln(d.stdout, m)
		}
		return fmt.Errorf("session %s matched %d transcripts, expected exactly one", sessionID, len(matches))
	}

	path := matches[0]
	if grepPattern == "" {
		fmt.Fprintln(d.stdout, path)
		return nil
	}

	lines, err := grepFileLines(path, grepPattern)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(d.stdout, line)
	}
	return nil
}

// grepFileLines returns path's lines matching pattern as a case-insensitive
// Go regexp (RE2 syntax) match, independent of claudehistory.GrepTranscripts'
// rg-based (Rust regex) matching — the two are close for simple patterns but
// not a byte-for-byte semantic match.
func grepFileLines(path, pattern string) ([]string, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid --grep pattern: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		if re.MatchString(scanner.Text()) {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return lines, nil
}
