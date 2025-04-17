package service

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/andresrobam/leggo/config"
	"github.com/andresrobam/leggo/log"
	tea "github.com/charmbracelet/bubbletea/v2"
)

type State int

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateStopping
)

type LineType int

const (
	LineTypeStdout LineType = iota
	LineTypeStderr
	LineTypeSysout
	LineTypeSyserr
)

type Service struct {
	Key                string
	Name               string
	Path               string
	Commands           []Command
	State              State
	cmd                *exec.Cmd
	outPipe            *io.ReadCloser
	errPipe            *io.ReadCloser
	Program            *tea.Program
	StateMutex         sync.RWMutex
	TermAttemptCount   int
	Pid                int
	ActiveCommandIndex int
	Configuration      *config.Config
	Log                *log.Log
	Healthcheck        Healthcheck
	WaitList           []string
}

var Services map[string]*Service
var Locks map[string]*sync.Mutex

func New(key string, name string, path string, commands []Command, configuration *config.Config, healthcheck Healthcheck) Service {
	return Service{
		Key:           key,
		Name:          name,
		Path:          path,
		Commands:      commands,
		Configuration: configuration,
		Log:           log.New(configuration),
		Healthcheck:   healthcheck,
	}
}

type ServiceStoppedMsg struct {
	Service string
}

type ServiceStoppingMsg struct {
	Service string
}

type ServiceStartedMsg struct {
	Service string
}

type StartServiceMsg struct {
	Service string
}

type ContentUpdateMsg struct{}

func (s *Service) addOutput(addition string, endLine bool, lineType LineType) {
	s.Log.AddContent(addition, endLine)
}

func (s *Service) addStdout(addition string, endLine bool) {
	s.addOutput(addition, endLine, LineTypeStdout)
}

func (s *Service) addSterr(addition string, endLine bool) {
	s.addOutput(addition, endLine, LineTypeStderr)
}

func (s *Service) addSyserrLine(addition string) {
	s.addOutput(addition, true, LineTypeSyserr)
}

func (s *Service) addSysoutLine(addition string) {
	s.addOutput(addition, true, LineTypeSysout)
}

func writeFromPipe(pipe *io.ReadCloser, isErrorPipe bool, s *Service, wg *sync.WaitGroup) {
	buf := bufio.NewReader(*pipe)
	for {
		line, isPrefix, err := buf.ReadLine()
		if err == io.EOF {
			wg.Done()
			return
		} else if err != nil {
			if err.Error() != "read |0: file already closed" {
				bufname := "stdout"
				if isErrorPipe {
					bufname = "stderr"
				}
				s.addSyserrLine(fmt.Sprintf("Error reading %s buffer: %s", bufname, err))
			}
			wg.Done()
			return
		} else {
			if isErrorPipe {
				s.addSterr(string(line), !isPrefix)
			} else {
				s.addStdout(string(line), !isPrefix)
			}
		}
	}
}
