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
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}

func Kill(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}

func ShouldKillMatchingRegex() []string {
	return []string{}
}
