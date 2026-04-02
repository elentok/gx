package git

import "os/exec"

func DeltaAvailable() bool {
	_, err := exec.LookPath("delta")
	return err == nil
}
