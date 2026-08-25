package cli

import "syscall"

// processAlive reports whether a process with the given pid is currently running.
// Signal 0 does the kernel's existence/permission check without delivering a
// signal: a nil error means it's ours and alive; EPERM means alive but owned by
// someone else (still running).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// terminateProcess asks the process to shut down gracefully (SIGTERM); watch
// catches it and stops its loops cleanly.
func terminateProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}
