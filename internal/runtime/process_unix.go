//go:build !windows

package runtime

import (
	"os/exec"
	"syscall"
)

// npm's Codex entry point is a Node launcher. Kill its process group as well
// as the launcher so cancellation cannot leave a model connection running.
func isolateProcess(cmd *exec.Cmd)  { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcessTree(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
