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

func GracefulStop(process *os.Process) error {
	d, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	p, err := d.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}
	r, _, err := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(process.Pid))
	if r == 0 {
		return err
	}
	return nil
}

func Kill(process *os.Process) error {
	return exec.Command("taskkill", "/t", "/f", "/pid", strconv.Itoa(process.Pid)).Run()
}

func ShouldKillMatchingRegex() []string {
	return []string{"^(\\.|\\.\\/)?(gradle|mvn)w?.*", "^javaw? .*"}
}
