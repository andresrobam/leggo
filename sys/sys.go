package sys

import "syscall"

func SendCtrlBreak(pid int) error {
	d, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	p, err := d.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}
	r, _, err := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
	if r == 0 {
		return err
	}
	return nil
}
