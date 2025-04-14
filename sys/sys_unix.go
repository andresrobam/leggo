//go:build unix

package sys

import (
	"os"
	"syscall"
)

func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func GracefulStop(process *os.Process) error {
	return process.Signal(syscall.SIGKILL)
}

func Kill(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
