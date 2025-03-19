//go:build windows

package sys

import (
	"os"
	"syscall"
)

const DefaultCommand = "cmd"
const DefaultCommandFlag = "/C"

func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func EndProcess(process *os.Process, termAttempt int) error {
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
