package service

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
	Lock     string
	Requires []string
	Kill     bool
}

type Healthcheck struct {
	Command string
	Period  int
}

func (s *Service) StartService() {

	if s.State == StateRunning || s.State == StateStopping || (s.State == StateStarting && s.cmd != nil) {
		return
	}

	if s.State == StateStopped && s.ActiveCommandIndex == 0 {
		for i := range s.Commands {
			s.State = StateStarting
			for _, requiredService := range s.Commands[i].Requires {
				if !slices.Contains(s.WaitList, requiredService) && Services[requiredService].State != StateRunning {
					s.WaitList = append(s.WaitList, requiredService)
					s.addSysoutLine(fmt.Sprintf("Starting required service: %s", requiredService))
					defer func() {
						go s.Program.Send(StartServiceMsg{Service: requiredService})
					}()
				}
			}
		}
	}

	c := s.Commands[s.ActiveCommandIndex]

	for _, requiredService := range c.Requires {
		if slices.Contains(s.WaitList, requiredService) {
			s.addSysoutLine(fmt.Sprintf("Waiting for required services to start: %s", strings.Join(s.WaitList, ", ")))
			return
		}
	}

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
		if s.Healthcheck.Command != "" {
			go s.CheckHealth()
		} else {
			s.State = StateRunning
			go s.Program.Send(ServiceStartedMsg{Service: s.Key})
		}
	}
	go handleRunningProcess(wg, &outPipe, s, &errPipe)
}

func (s *Service) CheckHealth() {

	if s.State != StateStarting {
		return
	}

	hc := exec.Command(s.Configuration.CommandExecutor, s.Configuration.CommandArgument, s.Healthcheck.Command)
	hc.SysProcAttr = sys.GetSysProcAttr()
	hc.Dir = s.Path
	s.addSysoutLine(fmt.Sprintf("Running healthcheck \"%s\"", s.Healthcheck.Command))
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
			go s.Program.Send(ServiceStartedMsg{Service: s.Key})
			return
		}
		s.addSyserrLine(fmt.Sprintf("Healthcheck failed with exit code: %d", hc.ProcessState.ExitCode()))
	}
	healthCheckPeriod := s.Healthcheck.Period
	if healthCheckPeriod == 0 {
		healthCheckPeriod = 1
	}
	<-time.After(time.Duration(healthCheckPeriod) * time.Second)
	s.CheckHealth()

}

func (s *Service) DoneWaiting(service string) {
	s.StateMutex.Lock()
	defer s.StateMutex.Unlock()
	i := slices.Index(s.WaitList, service)
	if i == -1 {
		return
	}
	s.WaitList = slices.Delete(s.WaitList, i, i+1)
	if len(s.WaitList) == 0 {
		s.addSysoutLine("All dependencies for are up, starting")
		if s.State == StateStarting {
			s.StartService()
		}
	}
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
	s.cmd = nil
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
		s.WaitList = []string{}
		s.State = StateStopped
		go s.Program.Send(ServiceStoppedMsg{Service: s.Key})
	}
	s.StateMutex.Unlock()
}

func (s *Service) EndService() {
	s.TermAttemptCount++
	s.State = StateStopping
	s.WaitList = []string{}
	s.addSysoutLine("Closing process")
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.end(); err != nil {
			s.addSyserrLine(fmt.Sprintf("Error closing process: %s", err))
		} else {
			go s.Program.Send(ServiceStoppingMsg{Service: s.Key})
		}
	} else {
		s.TermAttemptCount = 0
		s.WaitList = []string{}
		s.State = StateStopped
		go s.Program.Send(ServiceStoppedMsg{Service: s.Key})
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
	}
	return sys.GracefulStop(s.cmd.Process)
}
