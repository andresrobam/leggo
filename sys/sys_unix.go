//go:build unix

package sys

import (
	"os"
	"syscall"
)

const DefaultCommand = "/bin/bash"
const DefaultCommandFlag = "-c"

func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func EndProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
