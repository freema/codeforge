package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// configureGracefulKill makes context cancellation terminate the CLI's whole
// process group with SIGTERM first, escalating to SIGKILL after a grace
// period. exec.CommandContext's default SIGKILLs only the direct child, which
// orphans helpers the CLI spawned and gives it no chance to clean up.
func configureGracefulKill(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the whole process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	// If the process ignores SIGTERM, Go SIGKILLs the direct child after this
	// delay. It also bounds Wait when orphans keep the output pipes open.
	cmd.WaitDelay = 15 * time.Second
}
