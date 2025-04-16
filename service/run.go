package service

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/andresrobam/leggo/sys"
)

var dockerAnsiReplacement = true

var isDockerComposeRegex = regexp.MustCompile("^ *docker[ -]compose +.*$")
var hasAnsiFlagRegex = regexp.MustCompile(`(^\s*docker[ -]compose +.*--ansi)(=| +)(\S+)(.*$)`)
var ansiFlagAddRegex = regexp.MustCompile("(^ *docker[ -])(compose)( +.*$)")

func forceDockerComposeAnsi(command string) string {
	if isDockerComposeRegex.MatchString(command) {
		if hasAnsiFlagRegex.MatchString(command) {
			// change ansi param value to "always"
			return hasAnsiFlagRegex.ReplaceAllString(command, "$1=always$4")
		} else {
			// add ansi param with value "always"
			return ansiFlagAddRegex.ReplaceAllString(command, "$1$2 --ansi=always$3")
		}
	}
	return command
}

func (s *Service) transform(command string) string {
	if s.Configuration.ForceDockerComposeAnsi {
		return forceDockerComposeAnsi(command)
	}
	return command
}

type Command struct {
	Command  string
	Path     string
	Unique   string   // TODO: implement
	Requires []string // TODO: implement
	Kill     bool
}

func (s *Service) StartService() {
	c := s.Commands[s.ActiveCommandIndex]

	command := s.transform(c.Command)

	s.cmd = exec.Command(s.Configuration.CommandExecutor, s.Configuration.CommandArgument, command)
	s.cmd.SysProcAttr = sys.GetSysProcAttr()

	if c.Path != "" {
		if filepath.IsAbs(c.Path) {
			s.cmd.Dir = c.Path
		} else {
			var pathErr error
			s.cmd.Dir, pathErr = filepath.Abs(filepath.Join(s.Path, c.Path))
			if pathErr != nil {
				s.addSyserrLine(fmt.Sprintf("Error: getting absolute path %s", pathErr))
				return
			}
		}
	} else {
		s.cmd.Dir = s.Path
	}
	pathMessage := fmt.Sprintf(" in %s", s.Path)

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
	s.State = StateStarting
	s.addSysoutLine(fmt.Sprintf("Process started with PID: %d", s.Pid))

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go writeFromPipe(&outPipe, false, s, wg)
	go writeFromPipe(&errPipe, true, s, wg)

	if s.ActiveCommandIndex == len(s.Commands)-1 {
		if s.HealthCheck != "" {
			go s.CheckHealth()
		} else {
			s.State = StateRunning
			go s.Program.Send(ServiceStartedMsg{})
		}
	}
	go handleRunningProcess(wg, &outPipe, s, &errPipe)
}

func (s *Service) CheckHealth() {

	if s.State != StateStarting {
		return
	}

	hc := exec.Command(s.Configuration.CommandExecutor, s.Configuration.CommandArgument, s.HealthCheck)
	hc.SysProcAttr = sys.GetSysProcAttr()
	hc.Dir = s.Path
	s.addSysoutLine(fmt.Sprintf("Running healthcheck \"%s\"", s.HealthCheck))
	if err := hc.Run(); err != nil {
		s.addSyserrLine(fmt.Sprintf("Error running healthcheck: %s", err))
	} else {
		if hc.ProcessState.ExitCode() == 0 {
			s.StateMutex.Lock()
			defer s.StateMutex.Unlock()
			if s.State != StateStarting {
				return
			}
			s.addSysoutLine("Healthcheck passed")
			s.State = StateRunning
			go s.Program.Send(ServiceStartedMsg{service: s.Key})
			return
		}
		s.addSyserrLine(fmt.Sprintf("Healthcheck failed with exit code: %d", hc.ProcessState.ExitCode()))
	}
	healthCheckPeriod := s.HealthCheckPeriod
	if healthCheckPeriod == 0 {
		healthCheckPeriod = 1
	}
	<-time.After(time.Duration(healthCheckPeriod) * time.Second)
	s.CheckHealth()

}

func handleRunningProcess(wg *sync.WaitGroup, outPipe *io.ReadCloser, s *Service, errPipe *io.ReadCloser) {

	wg.Wait()
	s.StateMutex.Lock()
	wasStopping := s.State == StateStopping
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
		go s.Program.Send(ServiceStoppedMsg{service: s.Key})
	}
	s.StateMutex.Unlock()
}

func (s *Service) EndService() {
	s.TermAttemptCount++
	s.State = StateStopping
	s.addSysoutLine("Closing process")
	if err := s.end(); err != nil {
		s.addSyserrLine(fmt.Sprintf("Error closing process: %s", err))
	} else {
		go s.Program.Send(ServiceStoppingMsg{service: s.Key})
	}
}

func (c Command) shouldKill() bool {
	if c.Kill {
		return true
	}
	for _, regex := range sys.ShouldKillMatchingRegex() {
		if regexp.MustCompile(regex).MatchString(c.Command) {
			return true
		}
	}
	return false
}

func (s *Service) end() error {
	if s.Commands[s.ActiveCommandIndex].shouldKill() || s.TermAttemptCount > 2 {
		return sys.Kill(s.cmd.Process)
	} else {
		return sys.GracefulStop(s.cmd.Process)
	}
}
