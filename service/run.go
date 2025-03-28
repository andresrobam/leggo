package service

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"

	"github.com/andresrobam/leggo/sys"
)

var dockerAnsiReplacement = true

var isDockerComposeRegex = regexp.MustCompile("^ *docker[ -]compose +.*$")
var hasAnsiFlagRegex = regexp.MustCompile(`(^\s*docker[ -]compose +.*--ansi)(=| +)(\S+)(.*$)`)
var ansiFlagAddRegex = regexp.MustCompile("(^ *docker[ -])(compose)( +.*$)")

func forceDockerComposeAnsi(command *string) string {
	if isDockerComposeRegex.MatchString(*command) {
		if hasAnsiFlagRegex.MatchString(*command) {
			// change ansi param value to "always"
			return hasAnsiFlagRegex.ReplaceAllString(*command, "$1=always$4")
		} else {
			// add ansi param with value "always"
			return ansiFlagAddRegex.ReplaceAllString(*command, "$1$2 --ansi=always$3")
		}
	}
	return *command
}

func (s *Service) StartService() {
	var command string
	if s.Configuration.ForceDockerComposeAnsi {
		command = forceDockerComposeAnsi(&s.Commands[s.ActiveCommandIndex])
	} else {
		command = s.Commands[s.ActiveCommandIndex]
	}
	s.cmd = exec.Command(s.Configuration.CommandExecutor, s.Configuration.CommandArgument, command)
	s.cmd.SysProcAttr = sys.GetSysProcAttr()
	var pathMessage string
	if s.Path != "" {
		s.cmd.Dir = s.Path
		pathMessage = fmt.Sprintf(" in %s", s.Path)
	}
	outPipe, err := s.cmd.StdoutPipe()
	if err != nil {
		s.addSyserrLine(fmt.Sprintf("Error: opening stdout pipe %s", err))
		return
	}
	s.outPipe = &outPipe

	errPipe, err := s.cmd.StderrPipe()
	if err != nil {
		s.addSyserrLine(fmt.Sprintf("Error: opening stderr pipe %s", err))
		return
	}
	s.errPipe = &errPipe

	s.addSysoutLine(fmt.Sprintf("Running command \"%s\"%s", command, pathMessage))
	if err := s.cmd.Start(); err != nil {
		s.addSyserrLine(fmt.Sprintf("Error running command: %s", err))
		return
	}
	s.Pid = s.cmd.Process.Pid
	s.addSysoutLine(fmt.Sprintf("Process started with PID: %d", s.Pid))

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go writeFromPipe(&outPipe, false, s, wg)
	go writeFromPipe(&errPipe, true, s, wg)
	s.State = StateRunning
	go s.Program.Send(ServiceStartedMsg{})
	go handleRunningProcess(wg, &outPipe, s, &errPipe)
}

func handleRunningProcess(wg *sync.WaitGroup, outPipe *io.ReadCloser, s *Service, errPipe *io.ReadCloser) {

	wg.Wait()
	s.StateMutex.Lock()
	wasStopping := s.State == StateStopping
	s.State = StateStopping
	if err := (*outPipe).Close(); err != nil && err.Error() != "close |0: file already closed" {
		s.addSyserrLine(fmt.Sprintf("Error closing stdout pipe: %s", err))
	}
	if err := (*errPipe).Close(); err != nil && err.Error() != "close |0: file already closed" {
		s.addSyserrLine(fmt.Sprintf("Error closing stderr pipe: %s", err))
	}
	s.cmd.Wait()
	s.Pid = 0

	exitCode := s.cmd.ProcessState.ExitCode()
	message := fmt.Sprintf("Process finished with exit code: %d", exitCode)
	s.addSysoutLine(message)

	var runNextCommand bool
	if !wasStopping && exitCode == 0 {
		s.ActiveCommandIndex++
		if s.ActiveCommandIndex >= len(s.Commands) {
			s.ActiveCommandIndex = 0
		}
		if s.ActiveCommandIndex != 0 {
			runNextCommand = true
		}
	} else {
		s.ActiveCommandIndex = 0
	}

	s.TermAttemptCount = 0
	if runNextCommand {
		go s.StartService()
	} else {
		s.State = StateStopped
		go s.Program.Send(ServiceStoppedMsg{})
	}
	s.StateMutex.Unlock()
}

func (s *Service) EndService() {
	s.TermAttemptCount++
	s.State = StateStopping
	s.addSysoutLine("Closing process")
	if err := sys.EndProcess(s.cmd.Process, s.TermAttemptCount); err != nil {
		s.addSyserrLine(fmt.Sprintf("Error closing process: %s", err))
	} else {
		go s.Program.Send(ServiceStoppingMsg{})
	}
}
