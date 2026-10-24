package brief

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runForwardingSignals runs cmd, passing interrupts sent to brief on to the
// child and waiting for it, so the caller can still write the log and index.
func runForwardingSignals(cmd *exec.Cmd) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	close(done)
	return err
}
