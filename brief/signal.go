package brief

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runForwardingSignals runs cmd, passing interrupts sent to brief on to the
// child and waiting for it, so the caller can still write the log and index.
// Closing stop (may be nil) interrupts the child the same way. `go test` does
// not pass an interrupt on to its test binary, so a stoppable child gets its
// own process group and every signal goes to the whole group.
func runForwardingSignals(cmd *exec.Cmd, stop <-chan struct{}) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	grouped := stop != nil
	if grouped {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	send := func(s os.Signal) {
		if sig, ok := s.(syscall.Signal); ok && grouped {
			_ = syscall.Kill(-cmd.Process.Pid, sig)
			return
		}
		_ = cmd.Process.Signal(s)
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				send(s)
			case <-stop:
				send(os.Interrupt)
				stop = nil
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	close(done)
	return err
}
