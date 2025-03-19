package service

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea/v2"
)

type State int

const (
	StateStopped State = iota
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
	Name             string
	Path             string
	Content          string
	Commands         []string
	Active           bool
	State            State
	cmd              *exec.Cmd
	outPipe          *io.ReadCloser
	errPipe          *io.ReadCloser
	Program          *tea.Program
	StateMutex       sync.RWMutex
	atStartOfLine    bool
	TermAttemptCount int
	ContentUpdated   atomic.Bool
	YOffset          int
	WasAtBottom      bool
}

func New(name string, path string, commands []string) Service {
	return Service{
		Name:          name,
		Path:          path,
		Commands:      commands,
		atStartOfLine: true,
	}
}

type ContentUpdateMsg struct{}

type ServiceStoppedMsg struct{}

type ServiceStoppingMsg struct{}

type ServiceStartedMsg struct{}

const maxBytes int = 50 * 1024 * 1024

func (s *Service) clearOldLines() {
	exceededBytes := len(s.Content) - maxBytes
	if exceededBytes > 0 {
		s.Content = s.Content[exceededBytes:]
	}
}

func (s *Service) addOutput(addition *string, endLine bool, lineType LineType) {
	// if atStartOfLine maybe add timestamps and shit
	s.Content += *addition
	if endLine {
		s.Content += "\n"
	} else {
		s.atStartOfLine = false
	}
	//render based on lineType
	s.clearOldLines()
	s.ContentUpdated.Store(true)
}

func (s *Service) addStdout(addition string, endLine bool) {
	s.addOutput(&addition, endLine, LineTypeStdout)
}

func (s *Service) addSterr(addition string, endLine bool) {
	s.addOutput(&addition, endLine, LineTypeStderr)
}

func (s *Service) addSyserrLine(addition string) {
	s.addOutput(&addition, true, LineTypeSyserr)
}

func (s *Service) addSysoutLine(addition string) {
	s.addOutput(&addition, true, LineTypeSysout)
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
