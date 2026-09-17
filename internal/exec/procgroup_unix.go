//go:build unix

package exec

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so the whole tree
// can be signalled together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills the child's entire process group.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// Fall back to the direct child if the group is already gone.
		return cmd.Process.Kill()
	}
	return nil
}
