package brief

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
)

// MaxRepeatParallel is the most runs `--repeat` starts at once.
const MaxRepeatParallel = 4

// Repeat runs the tool n times, parallel at a time, one tool process per run
// and one log per run. It prints one summary line, then the log path of the
// first failing run. Exit: 0 all passed, 1 any failed, 2 any run could not
// execute.
func Repeat(tool Tool, o Options, n, parallel int) int {
	type result struct {
		exit int
		text string
	}
	results := make([]result, n)
	sem := make(chan struct{}, max(parallel, 1))
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var buf bytes.Buffer
			ro := o
			ro.Out = &buf
			exit, _ := RunOutcome(tool, ro)
			results[i] = result{exit, strings.TrimRight(buf.String(), "\n")}
		}()
	}
	wg.Wait()

	var passed, failed, cantRun int
	firstLog, firstErr := "", ""
	for _, r := range results {
		lastLine := r.text[strings.LastIndex(r.text, "\n")+1:]
		switch r.exit {
		case ExitPass:
			passed++
		case ExitFail:
			failed++
			if firstLog == "" {
				firstLog = lastLine
			}
		default:
			cantRun++
			if firstErr == "" {
				firstErr = lastLine
			}
		}
	}
	line := fmt.Sprintf("%d runs: %d passed, %d failed", n, passed, failed)
	if cantRun > 0 {
		line += fmt.Sprintf(", %d could not run", cantRun)
	}
	fmt.Fprintln(o.Out, line)
	if firstErr != "" {
		fmt.Fprintln(o.Out, firstErr)
	}
	if firstLog != "" {
		fmt.Fprintln(o.Out, firstLog)
	}
	switch {
	case cantRun > 0:
		return ExitCantRun
	case failed > 0:
		return ExitFail
	}
	return ExitPass
}
