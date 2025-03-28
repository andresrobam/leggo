//go:build unix

package sys

import (
	"os"
	"syscall"
)

func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func EndProcess(process *os.Process, termAttempt int) error {
	return process.Signal(syscall.SIGTERM)
}
