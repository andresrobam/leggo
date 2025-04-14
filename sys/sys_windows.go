//go:build windows

package sys

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func EndProcess(process *os.Process, termAttempt int) error {
	return exec.Command("taskkill", "/t", "/f", "/pid", strconv.Itoa(process.Pid)).Run()
}
